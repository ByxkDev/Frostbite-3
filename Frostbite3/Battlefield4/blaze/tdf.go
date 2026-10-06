package blaze

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var ErrLegacyWireNotPorted = errors.New("tdf: legacy wire primitive not ported")

type TdfBaseType byte

const (
	TdfTypeInteger         TdfBaseType = 0x0 
	TdfTypeMin             TdfBaseType = 0x0
	TdfTypeString          TdfBaseType = 0x1 
	TdfTypeBinary          TdfBaseType = 0x2 
	TdfTypeStruct          TdfBaseType = 0x3 
	TdfTypeList            TdfBaseType = 0x4 
	TdfTypeMap             TdfBaseType = 0x5 
	TdfTypeUnion           TdfBaseType = 0x6 
	TdfTypeVariable        TdfBaseType = 0x7
	TdfTypeBlazeObjectType TdfBaseType = 0x8 
	TdfTypeBlazeObjectID   TdfBaseType = 0x9 
	TdfTypeFloat           TdfBaseType = 0xA 
	TdfTypeTimeValue       TdfBaseType = 0xB 
	TdfTypeMax             TdfBaseType = 0xC
)

func (t TdfBaseType) String() string {
	switch t {
	case TdfTypeInteger:
		return "TDF_TYPE_INTEGER"
	case TdfTypeString:
		return "TDF_TYPE_STRING"
	case TdfTypeBinary:
		return "TDF_TYPE_BINARY"
	case TdfTypeStruct:
		return "TDF_TYPE_STRUCT"
	case TdfTypeList:
		return "TDF_TYPE_LIST"
	case TdfTypeMap:
		return "TDF_TYPE_MAP"
	case TdfTypeUnion:
		return "TDF_TYPE_UNION"
	case TdfTypeVariable:
		return "TDF_TYPE_VARIABLE"
	case TdfTypeBlazeObjectType:
		return "TDF_TYPE_BLAZE_OBJECT_TYPE"
	case TdfTypeBlazeObjectID:
		return "TDF_TYPE_BLAZE_OBJECT_ID"
	case TdfTypeFloat:
		return "TDF_TYPE_FLOAT"
	case TdfTypeTimeValue:
		return "TDF_TYPE_TIMEVALUE"
	case TdfTypeMax:
		return "TDF_TYPE_MAX"
	}
	return fmt.Sprintf("TDF_TYPE_UNKNOWN(0x%02X)", byte(t))
}

type TdfLegacyBaseType byte

const (
	LegacyTypeStruct TdfLegacyBaseType = 0x0
	LegacyTypeString TdfLegacyBaseType = 0x1
	LegacyTypeInt8   TdfLegacyBaseType = 0x2
	LegacyTypeUInt8  TdfLegacyBaseType = 0x3
	LegacyTypeInt16  TdfLegacyBaseType = 0x4
	LegacyTypeUInt16 TdfLegacyBaseType = 0x5
	LegacyTypeInt32  TdfLegacyBaseType = 0x6
	LegacyTypeUInt32 TdfLegacyBaseType = 0x7
	LegacyTypeInt64  TdfLegacyBaseType = 0x8
	LegacyTypeUInt64 TdfLegacyBaseType = 0x9
	LegacyTypeArray  TdfLegacyBaseType = 0xA
	LegacyTypeBlob   TdfLegacyBaseType = 0xB
	LegacyTypeMap    TdfLegacyBaseType = 0xC
	LegacyTypeUnion  TdfLegacyBaseType = 0xD
)

func (t TdfLegacyBaseType) String() string {
	names := [...]string{
		"TYPE_STRUCT", "TYPE_STRING", "TYPE_INT8", "TYPE_UINT8", "TYPE_INT16",
		"TYPE_UINT16", "TYPE_INT32", "TYPE_UINT32", "TYPE_INT64", "TYPE_UINT64",
		"TYPE_ARRAY", "TYPE_BLOB", "TYPE_MAP", "TYPE_UNION",
	}
	if int(t) < len(names) {
		return names[t]
	}
	return fmt.Sprintf("TYPE_UNKNOWN(0x%02X)", byte(t))
}

type TimeValue struct {
	Time int64
}

type BlazeObjectType struct {
	Component uint16
	Type      uint16
}

func NewBlazeObjectType(component, typ uint16) BlazeObjectType {
	return BlazeObjectType{Component: component, Type: typ}
}

func (t BlazeObjectType) String() string {
	return fmt.Sprintf("%d/%d", t.Component, t.Type)
}

type BlazeObjectID struct {
	ID   int64
	Type BlazeObjectType
}

func NewBlazeObjectID(id int64, typ BlazeObjectType) BlazeObjectID {
	return BlazeObjectID{ID: id, Type: typ}
}

func (o BlazeObjectID) String() string {
	return fmt.Sprintf("%s/%d", o.Type, o.ID)
}

const UnionUnsetMember byte = 0x7F

type Union struct {
	ActiveMember byte
}

var tdfUnionValuTag = []byte{0xDA, 0x1B, 0x35, byte(TdfTypeStruct)}

var tdfLegacyUnionValuTag = []byte{0xDA, 0x1B, 0x35}

const TdfTagLength = 3

type TdfMember struct {
	Tag   string
	Bytes [TdfTagLength]byte
}

func NewTdfMember(tagString string) (TdfMember, error) {
	for i := 0; i < len(tagString); i++ {
		if tagString[i] > 0x7F {
			return TdfMember{}, fmt.Errorf("tdf: tag can only consist of ASCII characters from 32 to 95 (%q)", tagString)
		}
	}

	tag := strings.ToUpper(tagString)
	n := len(tag)

	if n > 4 || n <= 0 {
		return TdfMember{}, fmt.Errorf("tdf: tag length can be [1;4] (%q)", tagString)
	}

	for i := 0; i < n; i++ {
		if tag[i] < ' ' || tag[i] > '_' {
			return TdfMember{}, fmt.Errorf("tdf: tag can only consist of ASCII characters from 32 to 95 (%q)", tagString)
		}
	}

	if tag[0] > 'Z' {
		return TdfMember{}, fmt.Errorf("tdf: tag must begin with letter [A-Z] (%q)", tagString)
	}

	result := uint32(tag[0]-32) << 26
	if n > 1 {
		result |= uint32((tag[1]-32)&0x3F) << 20
		if n > 2 {
			result |= uint32((tag[2]-32)&0x3F) << 14
			if n > 3 {
				result |= uint32((tag[3]-32)&0x3F) << 8
			}
		}
	}

	return TdfMember{
		Tag:   tag,
		Bytes: [TdfTagLength]byte{byte(result >> 24), byte(result >> 16), byte(result >> 8)},
	}, nil
}

func MustTdfMember(tagString string) TdfMember {
	m, err := NewTdfMember(tagString)
	if err != nil {
		panic(err)
	}
	return m
}

func TdfMemberFromBytes(b [TdfTagLength]byte) TdfMember {
	num := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8

	var buf [4]byte
	length := 4

	if num&0x3F00 != 0 {
		buf[3] = byte(((num >> 8) & 0x3F) + 32)
	} else {
		length = 3
	}

	if v := (num >> 14) & 0x3F; v != 0 {
		buf[2] = byte(v + 32)
	} else {
		length = 2
	}

	if v := (num >> 20) & 0x3F; v != 0 {
		buf[1] = byte(v + 32)
	} else {
		length = 1
	}

	if v := num >> 26; v != 0 {
		buf[0] = byte(v + 32)
	} else {
		length = 0
	}

	return TdfMember{Tag: string(buf[:length]), Bytes: b}
}

func ReadTdfMember(r io.Reader) (*TdfMember, error) {
	var b [TdfTagLength]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return nil, err
	}
	m := TdfMemberFromBytes(b)
	return &m, nil
}

func (m TdfMember) String() string { return m.Tag }

var (
	byteType            = reflect.TypeOf(byte(0))
	timeValueType       = reflect.TypeOf(TimeValue{})
	blazeObjectTypeType = reflect.TypeOf(BlazeObjectType{})
	blazeObjectIDType   = reflect.TypeOf(BlazeObjectID{})
	unionMarkerType     = reflect.TypeOf(Union{})
)

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func indirect(v reflect.Value) (reflect.Value, bool) {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return v, false
		}
		v = v.Elem()
	}
	return v, true
}

func isNilValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map:
		return v.IsNil()
	}
	return false
}

func isUnionType(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type == unionMarkerType {
			return true
		}
	}
	return false
}

func tdfBaseTypeOf(t reflect.Type) TdfBaseType {
	t = derefType(t)

	switch t.Kind() {
	case reflect.Bool,
		reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int,
		reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		return TdfTypeInteger
	case reflect.Float32, reflect.Float64:
		return TdfTypeFloat
	case reflect.String:
		return TdfTypeString
	case reflect.Slice:
		if t.Elem() == byteType {
			return TdfTypeBinary
		}
		return TdfTypeList
	case reflect.Map:
		return TdfTypeMap
	case reflect.Interface:
		if t.NumMethod() == 0 {
			return TdfTypeVariable
		}
	case reflect.Struct:
		switch t {
		case timeValueType:
			return TdfTypeInteger
		case blazeObjectTypeType:
			return TdfTypeBlazeObjectType
		case blazeObjectIDType:
			return TdfTypeBlazeObjectID
		}
		if isUnionType(t) {
			return TdfTypeUnion
		}
		return TdfTypeStruct
	}

	return TdfTypeMax
}

func isPrimitiveBase(b TdfBaseType) bool {
	switch b {
	case TdfTypeInteger, TdfTypeString, TdfTypeBinary, TdfTypeFloat:
		return true
	}
	return false
}

func sortMapKeys(keys []reflect.Value) {
	if len(keys) < 2 {
		return
	}

	switch keys[0].Kind() {
	case reflect.String:
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		sort.Slice(keys, func(i, j int) bool { return keys[i].Int() < keys[j].Int() })
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		sort.Slice(keys, func(i, j int) bool { return keys[i].Uint() < keys[j].Uint() })
	case reflect.Float32, reflect.Float64:
		sort.Slice(keys, func(i, j int) bool { return keys[i].Float() < keys[j].Float() })
	case reflect.Bool:
		sort.Slice(keys, func(i, j int) bool { return !keys[i].Bool() && keys[j].Bool() })
	}
}

type tdfField struct {
	member TdfMember
	index  int
	name   string
}

type structInfo struct {
	declared []tdfField     
	sorted   []tdfField    
	byTag    map[string]int 
}

var (
	structInfoCache sync.Map 
	emptyStructInfo = &structInfo{byTag: map[string]int{}}
)

func getStructInfo(t reflect.Type) (*structInfo, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("tdf: %s is not a struct", t)
	}

	if cached, ok := structInfoCache.Load(t); ok {
		return cached.(*structInfo), nil
	}

	info := &structInfo{byTag: make(map[string]int)}

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)

		tag, ok := sf.Tag.Lookup("tdf")
		if !ok || tag == "" || tag == "-" {
			continue
		}

		if sf.PkgPath != "" {
			return nil, fmt.Errorf("tdf: field %s.%s has a tdf tag but is not exported", t.Name(), sf.Name)
		}

		member, err := NewTdfMember(tag)
		if err != nil {
			return nil, fmt.Errorf("tdf: %s.%s: %w", t.Name(), sf.Name, err)
		}

		if _, dup := info.byTag[member.Tag]; dup {
			return nil, fmt.Errorf("tdf: duplicate tag %q in %s", member.Tag, t.Name())
		}

		info.byTag[member.Tag] = i
		info.declared = append(info.declared, tdfField{member: member, index: i, name: sf.Name})
	}

	info.sorted = append([]tdfField(nil), info.declared...)
	sort.SliceStable(info.sorted, func(a, b int) bool {
		return info.sorted[a].member.Tag < info.sorted[b].member.Tag
	})

	structInfoCache.Store(t, info)
	return info, nil
}

type unionMember struct {
	active     byte
	index      int
	name       string
	structType reflect.Type
}

type unionInfo struct {
	activeIndex int
	members     []unionMember
	byActive    map[byte]int 
}

var unionInfoCache sync.Map 

func getUnionInfo(t reflect.Type) (*unionInfo, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("tdf: %s is not a union struct", t)
	}

	if cached, ok := unionInfoCache.Load(t); ok {
		return cached.(*unionInfo), nil
	}

	info := &unionInfo{activeIndex: -1, byActive: make(map[byte]int)}

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)

		if sf.Anonymous && sf.Type == unionMarkerType {
			info.activeIndex = i
			continue
		}

		raw, ok := sf.Tag.Lookup("tdfunion")
		if !ok {
			continue
		}

		if sf.PkgPath != "" {
			return nil, fmt.Errorf("tdf: union member %s.%s is not exported", t.Name(), sf.Name)
		}

		n, err := strconv.ParseUint(raw, 0, 8)
		if err != nil || byte(n) == UnionUnsetMember {
			return nil, fmt.Errorf("tdf: invalid tdfunion member number %q on %s.%s", raw, t.Name(), sf.Name)
		}

		if sf.Type.Kind() != reflect.Ptr || sf.Type.Elem().Kind() != reflect.Struct {
			return nil, fmt.Errorf("tdf: union member %s.%s must be a pointer to a struct", t.Name(), sf.Name)
		}

		if _, dup := info.byActive[byte(n)]; dup {
			return nil, fmt.Errorf("tdf: duplicate union member number %d in %s", n, t.Name())
		}

		info.byActive[byte(n)] = len(info.members)
		info.members = append(info.members, unionMember{
			active:     byte(n),
			index:      i,
			name:       sf.Name,
			structType: sf.Type.Elem(),
		})
	}

	if info.activeIndex < 0 {
		return nil, fmt.Errorf("tdf: %s does not embed blaze.Union", t)
	}

	unionInfoCache.Store(t, info)
	return info, nil
}

func unionActive(v reflect.Value, info *unionInfo) byte {
	return byte(v.Field(info.activeIndex).Field(0).Uint())
}

func unionSetActive(v reflect.Value, info *unionInfo, active byte) {
	v.Field(info.activeIndex).Field(0).SetUint(uint64(active))
}

func unionCurrent(v reflect.Value, info *unionInfo) (active byte, member reflect.Value, ok bool) {
	active = unionActive(v, info)
	if active == UnionUnsetMember {
		return active, reflect.Value{}, false
	}

	idx, found := info.byActive[active]
	if !found {
		return active, reflect.Value{}, false
	}

	f := v.Field(info.members[idx].index)
	if f.IsNil() {
		return active, reflect.Value{}, false
	}

	return active, f.Elem(), true
}

func unionValueOf(union interface{}) (reflect.Value, *unionInfo, error) {
	v := reflect.ValueOf(union)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}, nil, errors.New("tdf: nil union")
		}
		v = v.Elem()
	}

	if !v.IsValid() {
		return reflect.Value{}, nil, errors.New("tdf: invalid union")
	}

	info, err := getUnionInfo(v.Type())
	if err != nil {
		return reflect.Value{}, nil, err
	}

	return v, info, nil
}

func UnionGetValue(union interface{}) interface{} {
	v, info, err := unionValueOf(union)
	if err != nil {
		return nil
	}

	_, member, ok := unionCurrent(v, info)
	if !ok {
		return nil
	}

	return member.Addr().Interface()
}

func UnionGetValueName(union interface{}) string {
	v, info, err := unionValueOf(union)
	if err != nil {
		return ""
	}

	active := unionActive(v, info)
	if active == UnionUnsetMember {
		return ""
	}

	if idx, ok := info.byActive[active]; ok {
		return info.members[idx].name
	}
	return ""
}

func UnionActiveMemberType(union interface{}, active byte) reflect.Type {
	_, info, err := unionValueOf(union)
	if err != nil {
		return nil
	}

	if idx, ok := info.byActive[active]; ok {
		return info.members[idx].structType
	}
	return nil
}

func UnionActiveMemberName(union interface{}, active byte) string {
	_, info, err := unionValueOf(union)
	if err != nil {
		return ""
	}

	if idx, ok := info.byActive[active]; ok {
		return info.members[idx].name
	}
	return ""
}

func UnionSetValue(union interface{}, value interface{}) error {
	pv := reflect.ValueOf(union)
	if pv.Kind() != reflect.Ptr || pv.IsNil() {
		return errors.New("tdf: UnionSetValue needs a non-nil pointer to a union struct")
	}

	v := pv.Elem()
	info, err := getUnionInfo(v.Type())
	if err != nil {
		return err
	}

	if value == nil {
		for _, m := range info.members {
			f := v.Field(m.index)
			f.Set(reflect.Zero(f.Type()))
		}
		unionSetActive(v, info, UnionUnsetMember)
		return nil
	}

	val := reflect.ValueOf(value)
	ptr := val
	if val.Kind() != reflect.Ptr {
		p := reflect.New(val.Type())
		p.Elem().Set(val)
		ptr = p
	}

	found := false
	for _, m := range info.members {
		f := v.Field(m.index)
		if !found && f.Type() == ptr.Type() {
			f.Set(ptr)
			unionSetActive(v, info, m.active)
			found = true
		} else {
			f.Set(reflect.Zero(f.Type()))
		}
	}

	if !found {
		return fmt.Errorf("tdf: %s is not a member of union %s", ptr.Type(), v.Type())
	}
	return nil
}

func writeVarInt(buf *bytes.Buffer, v uint64) {
	if v < 0x40 {
		buf.WriteByte(byte(v))
		return
	}

	buf.WriteByte(byte(v&0x3F) | 0x80)
	v >>= 6

	for v >= 0x80 {
		buf.WriteByte(byte(v&0x7F) | 0x80)
		v >>= 7
	}

	buf.WriteByte(byte(v))
}

type tdfReader struct {
	data []byte
	pos  int
}

func (r *tdfReader) remaining() int { return len(r.data) - r.pos }

func (r *tdfReader) readByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *tdfReader) readBytes(n int) ([]byte, error) {
	if n < 0 || r.remaining() < n {
		return nil, io.ErrUnexpectedEOF
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func readVarInt(r *tdfReader) (uint64, error) {
	b, err := r.readByte()
	if err != nil {
		return 0, err
	}

	result := uint64(b & 0x3F)
	shift := uint(6)
	count := 1

	for b >= 0x80 {
		b, err = r.readByte()
		if err != nil {
			return 0, err
		}

		count++
		if count > 11 {
			return 0, errors.New("tdf: varint too long")
		}

		if shift < 64 {
			result |= uint64(b&0x7F) << shift
		}
		shift += 7
	}

	return result, nil
}

func readLengthPrefixed(r *tdfReader) ([]byte, error) {
	n, err := readVarInt(r)
	if err != nil {
		return nil, err
	}

	if n > uint64(r.remaining()) {
		return nil, io.ErrUnexpectedEOF
	}

	return r.readBytes(int(n))
}

func writeTdfString(buf *bytes.Buffer, s string) {
	writeVarInt(buf, uint64(len(s))+1)
	buf.WriteString(s)
	buf.WriteByte(0)
}

func writeTdfBlob(buf *bytes.Buffer, b []byte) {
	writeVarInt(buf, uint64(len(b)))
	buf.Write(b)
}

func writeTdfFloat(buf *bytes.Buffer, f float32) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], math.Float32bits(f))
	buf.Write(tmp[:])
}

func writeTdfBlazeObjectType(buf *bytes.Buffer, t BlazeObjectType) {
	writeVarInt(buf, uint64(t.Component))
	writeVarInt(buf, uint64(t.Type))
}

func writeTdfBlazeObjectID(buf *bytes.Buffer, o BlazeObjectID) {
	writeTdfBlazeObjectType(buf, o.Type)
	writeVarInt(buf, uint64(o.ID))
}

func readTdfBlazeObjectType(r *tdfReader) (BlazeObjectType, error) {
	component, err := readVarInt(r)
	if err != nil {
		return BlazeObjectType{}, err
	}

	typ, err := readVarInt(r)
	if err != nil {
		return BlazeObjectType{}, err
	}

	return BlazeObjectType{Component: uint16(component), Type: uint16(typ)}, nil
}

func readTdfBlazeObjectID(r *tdfReader) (BlazeObjectID, error) {
	typ, err := readTdfBlazeObjectType(r)
	if err != nil {
		return BlazeObjectID{}, err
	}

	id, err := readVarInt(r)
	if err != nil {
		return BlazeObjectID{}, err
	}

	return BlazeObjectID{ID: int64(id), Type: typ}, nil
}

func integerWireBits(v reflect.Value) uint64 {
	switch v.Kind() {
	case reflect.Int8:
		return uint64(uint8(v.Int()))
	case reflect.Int16:
		return uint64(uint16(v.Int()))
	case reflect.Int32:
		return uint64(uint32(v.Int()))
	case reflect.Int, reflect.Int64:
		return uint64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint()
	}
	return 0
}

func setIntegerBits(field reflect.Value, bits uint64) error {
	switch field.Kind() {
	case reflect.Bool:
		field.SetBool(bits != 0)
	case reflect.Int8:
		field.SetInt(int64(int8(bits)))
	case reflect.Int16:
		field.SetInt(int64(int16(bits)))
	case reflect.Int32:
		field.SetInt(int64(int32(bits)))
	case reflect.Int, reflect.Int64:
		field.SetInt(int64(bits))
	case reflect.Uint8:
		field.SetUint(uint64(uint8(bits)))
	case reflect.Uint16:
		field.SetUint(uint64(uint16(bits)))
	case reflect.Uint32:
		field.SetUint(uint64(uint32(bits)))
	case reflect.Uint, reflect.Uint64:
		field.SetUint(bits)
	case reflect.Struct:
		if field.Type() != timeValueType {
			return typeMismatch(field, TdfTypeInteger)
		}
		field.Set(reflect.ValueOf(TimeValue{Time: int64(bits)}))
	case reflect.Interface:
		if field.NumMethod() != 0 {
			return typeMismatch(field, TdfTypeInteger)
		}
		field.Set(reflect.ValueOf(int64(bits)))
	default:
		return typeMismatch(field, TdfTypeInteger)
	}
	return nil
}

func typeMismatch(field reflect.Value, base TdfBaseType) error {
	return fmt.Errorf("tdf: cannot decode wire type %s into Go type %s", base, field.Type())
}

type TdfFactory struct {
	mu            sync.RWMutex
	variableTypes map[uint32]reflect.Type
	typeIDs       map[reflect.Type]uint32
}

func NewTdfFactory() *TdfFactory {
	return &TdfFactory{
		variableTypes: make(map[uint32]reflect.Type),
		typeIDs:       make(map[reflect.Type]uint32),
	}
}

func (f *TdfFactory) RegisterTdfType(example interface{}, tdfID uint32) error {
	if example == nil {
		return errors.New("tdf: cannot register nil")
	}

	t := derefType(reflect.TypeOf(example))
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("tdf: %s is not a struct", t)
	}

	if isUnionType(t) {
		if _, err := getUnionInfo(t); err != nil {
			return err
		}
	} else if _, err := getStructInfo(t); err != nil {
		return err
	}

	if tdfID != 0 {
		f.mu.Lock()
		f.variableTypes[tdfID] = t
		f.typeIDs[t] = tdfID
		f.mu.Unlock()
	}

	return nil
}

func (f *TdfFactory) RegisterTdfTypes(types map[uint32]interface{}) error {
	for id, example := range types {
		if err := f.RegisterTdfType(example, id); err != nil {
			return err
		}
	}
	return nil
}

func (f *TdfFactory) GetType(tdfID uint32) reflect.Type {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.variableTypes[tdfID]
}

func (f *TdfFactory) GetTdfID(t reflect.Type) uint32 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.typeIDs[t]
}

func (f *TdfFactory) CreateEncoder(heat1Bug bool) *TdfEncoder {
	return &TdfEncoder{factory: f, heat1Bug: heat1Bug}
}

func (f *TdfFactory) CreateDecoder(heat1Bug bool) *TdfDecoder {
	return &TdfDecoder{factory: f, heat1Bug: heat1Bug}
}

func (f *TdfFactory) CreateLegacyEncoder() *TdfLegacyEncoder {
	return &TdfLegacyEncoder{factory: f}
}

func (f *TdfFactory) CreateLegacyDecoder() *TdfLegacyDecoder {
	return &TdfLegacyDecoder{factory: f}
}

type ITdfEncoder interface {
	Encode(obj interface{}) ([]byte, error)
	WriteTo(w io.Writer, obj interface{}) error
}

type ITdfDecoder interface {
	Decode(data []byte, out interface{}) error
	DecodeFrom(r io.Reader, out interface{}) error
}

var (
	_ ITdfEncoder = (*TdfEncoder)(nil)
	_ ITdfDecoder = (*TdfDecoder)(nil)
	_ ITdfEncoder = (*TdfLegacyEncoder)(nil)
	_ ITdfDecoder = (*TdfLegacyDecoder)(nil)
)

func prepareDecodeTarget(out interface{}) (reflect.Value, *structInfo, error) {
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return reflect.Value{}, nil, errors.New("tdf: Decode needs a non-nil pointer to a struct")
	}

	target := rv.Elem()
	if target.Kind() != reflect.Struct {
		return reflect.Value{}, nil, errors.New("tdf: Decode needs a pointer to a struct")
	}

	info, err := getStructInfo(target.Type())
	if err != nil {
		return reflect.Value{}, nil, err
	}

	return target, info, nil
}

func readAll(r io.Reader) ([]byte, error) {
	var b bytes.Buffer
	if _, err := b.ReadFrom(r); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

type TdfEncoder struct {
	factory  *TdfFactory
	heat1Bug bool
}

func (e *TdfEncoder) Encode(obj interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.encodeInto(&buf, obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (e *TdfEncoder) WriteTo(w io.Writer, obj interface{}) error {
	var buf bytes.Buffer
	if err := e.encodeInto(&buf, obj); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func (e *TdfEncoder) encodeInto(buf *bytes.Buffer, obj interface{}) error {
	v, ok := indirect(reflect.ValueOf(obj))
	if !ok || v.Kind() != reflect.Struct {
		return errors.New("tdf: Encode needs a struct or a non-nil pointer to a struct")
	}
	return e.writeStructBody(buf, v)
}

func (e *TdfEncoder) writeStructBody(buf *bytes.Buffer, v reflect.Value) error {
	info, err := getStructInfo(v.Type())
	if err != nil {
		return err
	}

	for _, f := range info.sorted {
		fv := v.Field(f.index)
		if isNilValue(fv) {
			continue
		}

		t := derefType(fv.Type())
		base := tdfBaseTypeOf(t)
		if base == TdfTypeMax {
			continue 
		}

		val, ok := indirect(fv)
		if !ok {
			continue
		}

		buf.Write(f.member.Bytes[:])
		buf.WriteByte(byte(base))

		if err := e.writeValue(buf, f.member, t, base, val, false); err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
	}

	return nil
}

func (e *TdfEncoder) writeValue(buf *bytes.Buffer, tag TdfMember, t reflect.Type, base TdfBaseType, v reflect.Value, listElement bool) error {
	switch base {
	case TdfTypeInteger:
		switch {
		case t.Kind() == reflect.Bool:
			if v.Bool() {
				buf.WriteByte(1)
			} else {
				buf.WriteByte(0)
			}
		case t == timeValueType:
			writeVarInt(buf, uint64(v.Interface().(TimeValue).Time))
		default:
			writeVarInt(buf, integerWireBits(v))
		}

	case TdfTypeString:
		writeTdfString(buf, v.String())

	case TdfTypeBinary:
		writeTdfBlob(buf, v.Bytes())

	case TdfTypeStruct:
		if v.Kind() != reflect.Struct {
			return fmt.Errorf("tdf: %s is not a struct", t)
		}
		if err := e.writeStructBody(buf, v); err != nil {
			return err
		}
		buf.WriteByte(0x00) 

	case TdfTypeList:
		return e.writeList(buf, tag, t, v)

	case TdfTypeMap:
		return e.writeMap(buf, tag, t, v)

	case TdfTypeUnion:
		return e.writeUnion(buf, v, listElement)

	case TdfTypeVariable:
		return e.writeVariable(buf, tag, v)

	case TdfTypeBlazeObjectType:
		writeTdfBlazeObjectType(buf, v.Interface().(BlazeObjectType))

	case TdfTypeBlazeObjectID:
		writeTdfBlazeObjectID(buf, v.Interface().(BlazeObjectID))

	case TdfTypeFloat:
		writeTdfFloat(buf, float32(v.Float()))

	default:
		return fmt.Errorf("tdf: no writer for %s", base)
	}

	return nil
}

func (e *TdfEncoder) writeList(buf *bytes.Buffer, tag TdfMember, t reflect.Type, v reflect.Value) error {
	et := derefType(t.Elem())
	eb := tdfBaseTypeOf(et)
	if eb == TdfTypeMax {
		return fmt.Errorf("tdf: list type %s not supported", et)
	}

	headerBase := eb
	if e.heat1Bug && (eb == TdfTypeUnion || eb == TdfTypeList || eb == TdfTypeMap) {
		headerBase = TdfTypeStruct
	}

	buf.WriteByte(byte(headerBase))
	writeVarInt(buf, uint64(v.Len()))

	for i := 0; i < v.Len(); i++ {
		item, ok := indirect(v.Index(i))
		if !ok {
			return fmt.Errorf("tdf: nil element at list index %d", i)
		}
		if err := e.writeValue(buf, tag, et, eb, item, true); err != nil {
			return err
		}
	}

	return nil
}

func (e *TdfEncoder) writeMap(buf *bytes.Buffer, tag TdfMember, t reflect.Type, v reflect.Value) error {
	kt := derefType(t.Key())
	vt := derefType(t.Elem())
	kb := tdfBaseTypeOf(kt)
	vb := tdfBaseTypeOf(vt)

	if kb == TdfTypeMax {
		return fmt.Errorf("tdf: map key type %s not supported", kt)
	}
	if vb == TdfTypeMax {
		return fmt.Errorf("tdf: map value type %s not supported", vt)
	}

	buf.WriteByte(byte(kb))
	buf.WriteByte(byte(vb))
	writeVarInt(buf, uint64(v.Len()))

	keys := v.MapKeys()
	sortMapKeys(keys)

	for _, k := range keys {
		kv, ok := indirect(k)
		if !ok {
			return errors.New("tdf: nil map key")
		}
		if err := e.writeValue(buf, tag, kt, kb, kv, false); err != nil {
			return err
		}

		vv, ok := indirect(v.MapIndex(k))
		if !ok {
			return errors.New("tdf: nil map value")
		}
		if err := e.writeValue(buf, tag, vt, vb, vv, false); err != nil {
			return err
		}
	}

	return nil
}

func (e *TdfEncoder) writeUnion(buf *bytes.Buffer, v reflect.Value, listElement bool) error {
	info, err := getUnionInfo(v.Type())
	if err != nil {
		return err
	}

	active, member, ok := unionCurrent(v, info)
	if !ok {
		buf.WriteByte(UnionUnsetMember)
		return nil
	}

	buf.WriteByte(active)

	if !listElement {
		buf.Write(tdfUnionValuTag)
	}

	if err := e.writeStructBody(buf, member); err != nil {
		return err
	}
	buf.WriteByte(0x00) 
	return nil
}

func (e *TdfEncoder) writeVariable(buf *bytes.Buffer, tag TdfMember, v reflect.Value) error {
	if v.Kind() != reflect.Interface || v.IsNil() {
		return errors.New("tdf: variable must hold a non-nil value")
	}

	inner, ok := indirect(v.Elem())
	if !ok {
		return errors.New("tdf: variable holds a nil pointer")
	}

	rt := inner.Type()
	base := tdfBaseTypeOf(rt)
	if base == TdfTypeMax || base == TdfTypeVariable {
		return fmt.Errorf("tdf: type %s not supported as a variable", rt)
	}

	buf.WriteByte(1) 
	writeVarInt(buf, uint64(e.factory.GetTdfID(rt)))
	buf.Write(tag.Bytes[:])
	buf.WriteByte(byte(base))

	if err := e.writeValue(buf, tag, rt, base, inner, false); err != nil {
		return err
	}

	buf.WriteByte(0x00) 
	return nil
}

type TdfDecoder struct {
	factory  *TdfFactory
	heat1Bug bool
}

func (d *TdfDecoder) Decode(data []byte, out interface{}) error {
	target, info, err := prepareDecodeTarget(out)
	if err != nil {
		return err
	}

	r := &tdfReader{data: data}
	for r.remaining() > 0 {
		if err := d.readTdf(r, target, info); err != nil {
			return err
		}
	}
	return nil
}

func (d *TdfDecoder) DecodeFrom(r io.Reader, out interface{}) error {
	data, err := readAll(r)
	if err != nil {
		return err
	}
	return d.Decode(data, out)
}

func (d *TdfDecoder) readTdf(r *tdfReader, target reflect.Value, info *structInfo) error {
	tagBytes, err := r.readBytes(TdfTagLength)
	if err != nil {
		return err
	}

	var tb [TdfTagLength]byte
	copy(tb[:], tagBytes)
	member := TdfMemberFromBytes(tb)

	bt, err := r.readByte()
	if err != nil {
		return err
	}

	var field reflect.Value
	if idx, ok := info.byTag[member.Tag]; ok {
		field = target.Field(idx)
	}

	if err := d.readValue(r, TdfBaseType(bt), field); err != nil {
		return fmt.Errorf("tag %q: %w", member.Tag, err)
	}
	return nil
}

func (d *TdfDecoder) readValue(r *tdfReader, base TdfBaseType, field reflect.Value) error {
	if field.IsValid() {
		switch field.Kind() {
		case reflect.Ptr:
			nv := reflect.New(field.Type().Elem())
			if err := d.readValue(r, base, nv.Elem()); err != nil {
				return err
			}
			field.Set(nv)
			return nil
		case reflect.Interface:
			if base == TdfTypeVariable {
				break
			}
			if field.NumMethod() != 0 || !isPrimitiveBase(base) {
				field = reflect.Value{}
			}
		}
	}

	switch base {
	case TdfTypeInteger:
		return d.readInteger(r, field)
	case TdfTypeString:
		return d.readString(r, field)
	case TdfTypeBinary:
		return d.readBlob(r, field)
	case TdfTypeStruct:
		return d.readStruct(r, field)
	case TdfTypeList:
		return d.readList(r, field)
	case TdfTypeMap:
		return d.readMap(r, field)
	case TdfTypeUnion:
		return d.readUnion(r, field)
	case TdfTypeVariable:
		return d.readVariable(r, field)
	case TdfTypeBlazeObjectType:
		return d.readBlazeObjectType(r, field)
	case TdfTypeBlazeObjectID:
		return d.readBlazeObjectID(r, field)
	case TdfTypeFloat:
		return d.readFloat(r, field)
	case TdfTypeTimeValue:
		return errors.New("tdf: TDF_TYPE_TIMEVALUE is not supported")
	}

	return fmt.Errorf("tdf: unknown base type 0x%02X", byte(base))
}

func (d *TdfDecoder) readInteger(r *tdfReader, field reflect.Value) error {
	bits, err := readVarInt(r)
	if err != nil {
		return err
	}

	if !field.IsValid() {
		return nil
	}
	return setIntegerBits(field, bits)
}

func (d *TdfDecoder) readString(r *tdfReader, field reflect.Value) error {
	raw, err := readLengthPrefixed(r)
	if err != nil {
		return err
	}

	if len(raw) > 0 && raw[len(raw)-1] == 0 {
		raw = raw[:len(raw)-1]
	}

	if !field.IsValid() {
		return nil
	}

	switch {
	case field.Kind() == reflect.String:
		field.SetString(string(raw))
	case field.Kind() == reflect.Interface && field.NumMethod() == 0:
		field.Set(reflect.ValueOf(string(raw)))
	default:
		return typeMismatch(field, TdfTypeString)
	}
	return nil
}

func (d *TdfDecoder) readBlob(r *tdfReader, field reflect.Value) error {
	raw, err := readLengthPrefixed(r)
	if err != nil {
		return err
	}

	if !field.IsValid() {
		return nil
	}

	cp := append([]byte(nil), raw...)

	switch {
	case field.Kind() == reflect.Slice && field.Type().Elem() == byteType:
		field.SetBytes(cp)
	case field.Kind() == reflect.Interface && field.NumMethod() == 0:
		field.Set(reflect.ValueOf(cp))
	default:
		return typeMismatch(field, TdfTypeBinary)
	}
	return nil
}

func (d *TdfDecoder) readFloat(r *tdfReader, field reflect.Value) error {
	b, err := r.readBytes(4)
	if err != nil {
		return err
	}

	f := math.Float32frombits(binary.BigEndian.Uint32(b))

	if !field.IsValid() {
		return nil
	}

	switch {
	case field.Kind() == reflect.Float32 || field.Kind() == reflect.Float64:
		field.SetFloat(float64(f))
	case field.Kind() == reflect.Interface && field.NumMethod() == 0:
		field.Set(reflect.ValueOf(f))
	default:
		return typeMismatch(field, TdfTypeFloat)
	}
	return nil
}

func (d *TdfDecoder) readBlazeObjectType(r *tdfReader, field reflect.Value) error {
	val, err := readTdfBlazeObjectType(r)
	if err != nil {
		return err
	}

	if !field.IsValid() {
		return nil
	}

	if field.Type() != blazeObjectTypeType {
		return typeMismatch(field, TdfTypeBlazeObjectType)
	}
	field.Set(reflect.ValueOf(val))
	return nil
}

func (d *TdfDecoder) readBlazeObjectID(r *tdfReader, field reflect.Value) error {
	val, err := readTdfBlazeObjectID(r)
	if err != nil {
		return err
	}

	if !field.IsValid() {
		return nil
	}

	if field.Type() != blazeObjectIDType {
		return typeMismatch(field, TdfTypeBlazeObjectID)
	}
	field.Set(reflect.ValueOf(val))
	return nil
}

func (d *TdfDecoder) readStructBody(r *tdfReader, sv reflect.Value, info *structInfo) error {
	for {
		b, err := r.readByte()
		if err != nil {
			return err
		}

		if b == 0x00 {
			return nil
		}

		r.pos-- 

		if err := d.readTdf(r, sv, info); err != nil {
			return err
		}
	}
}

func (d *TdfDecoder) readStruct(r *tdfReader, field reflect.Value) error {
	if !field.IsValid() {
		return d.readStructBody(r, reflect.Value{}, emptyStructInfo)
	}

	if field.Kind() != reflect.Struct {
		return typeMismatch(field, TdfTypeStruct)
	}

	info, err := getStructInfo(field.Type())
	if err != nil {
		return err
	}

	field.Set(reflect.Zero(field.Type()))
	return d.readStructBody(r, field, info)
}

func (d *TdfDecoder) readList(r *tdfReader, field reflect.Value) error {
	bt, err := r.readByte()
	if err != nil {
		return err
	}
	base := TdfBaseType(bt)

	count, err := readVarInt(r)
	if err != nil {
		return err
	}

	if count > uint64(r.remaining()) { 
		return io.ErrUnexpectedEOF
	}

	var elemType reflect.Type
	if field.IsValid() {
		if field.Kind() != reflect.Slice || field.Type().Elem() == byteType {
			return typeMismatch(field, TdfTypeList)
		}
		elemType = field.Type().Elem()
	}

	if d.heat1Bug && base == TdfTypeStruct && elemType != nil {
		et := derefType(elemType)
		switch {
		case isUnionType(et):
			base = TdfTypeUnion
		case et.Kind() == reflect.Slice && et.Elem() != byteType:
			base = TdfTypeList
		case et.Kind() == reflect.Map:
			base = TdfTypeMap
		}
	}

	if !field.IsValid() {
		for i := uint64(0); i < count; i++ {
			if err := d.readValue(r, base, reflect.Value{}); err != nil {
				return err
			}
		}
		return nil
	}

	list := reflect.MakeSlice(field.Type(), 0, int(count))
	for i := uint64(0); i < count; i++ {
		elem := reflect.New(elemType).Elem()
		if err := d.readValue(r, base, elem); err != nil {
			return err
		}
		list = reflect.Append(list, elem)
	}

	field.Set(list)
	return nil
}

func (d *TdfDecoder) readMap(r *tdfReader, field reflect.Value) error {
	kb, err := r.readByte()
	if err != nil {
		return err
	}

	vb, err := r.readByte()
	if err != nil {
		return err
	}

	count, err := readVarInt(r)
	if err != nil {
		return err
	}

	if count > uint64(r.remaining()) {
		return io.ErrUnexpectedEOF
	}

	if !field.IsValid() { 
		for i := uint64(0); i < count; i++ {
			if err := d.readValue(r, TdfBaseType(kb), reflect.Value{}); err != nil {
				return err
			}
			if err := d.readValue(r, TdfBaseType(vb), reflect.Value{}); err != nil {
				return err
			}
		}
		return nil
	}

	if field.Kind() != reflect.Map {
		return typeMismatch(field, TdfTypeMap)
	}

	mapType := field.Type()
	m := reflect.MakeMapWithSize(mapType, int(count))

	for i := uint64(0); i < count; i++ {
		k := reflect.New(mapType.Key()).Elem()
		if err := d.readValue(r, TdfBaseType(kb), k); err != nil {
			return err
		}

		v := reflect.New(mapType.Elem()).Elem()
		if err := d.readValue(r, TdfBaseType(vb), v); err != nil {
			return err
		}

		m.SetMapIndex(k, v)
	}

	field.Set(m)
	return nil
}

func (d *TdfDecoder) readUnion(r *tdfReader, field reflect.Value) error {
	am, err := r.readByte()
	if err != nil {
		return err
	}

	var info *unionInfo
	if field.IsValid() {
		if field.Kind() != reflect.Struct || !isUnionType(field.Type()) {
			return typeMismatch(field, TdfTypeUnion)
		}

		info, err = getUnionInfo(field.Type())
		if err != nil {
			return err
		}

		field.Set(reflect.Zero(field.Type()))
		unionSetActive(field, info, UnionUnsetMember)
	}

	if am == UnionUnsetMember {
		return nil
	}

	if r.remaining() >= len(tdfUnionValuTag) &&
		bytes.Equal(r.data[r.pos:r.pos+len(tdfUnionValuTag)], tdfUnionValuTag) {
		r.pos += len(tdfUnionValuTag)
	}

	if !field.IsValid() {
		return d.readStructBody(r, reflect.Value{}, emptyStructInfo)
	}

	idx, ok := info.byActive[am]
	if !ok { 
		return d.readStructBody(r, reflect.Value{}, emptyStructInfo)
	}

	m := info.members[idx]
	sinfo, err := getStructInfo(m.structType)
	if err != nil {
		return err
	}

	nv := reflect.New(m.structType)
	if err := d.readStructBody(r, nv.Elem(), sinfo); err != nil {
		return err
	}

	field.Field(m.index).Set(nv)
	unionSetActive(field, info, am)
	return nil
}

func (d *TdfDecoder) readVariable(r *tdfReader, field reflect.Value) error {
	present, err := r.readByte()
	if err != nil {
		return err
	}

	if present == 0 {
		if field.IsValid() && field.Kind() == reflect.Interface {
			field.Set(reflect.Zero(field.Type()))
		}
		return nil
	}

	rawID, err := readVarInt(r)
	if err != nil {
		return err
	}

	if _, err := r.readBytes(TdfTagLength); err != nil { 
		return err
	}

	bt, err := r.readByte()
	if err != nil {
		return err
	}
	base := TdfBaseType(bt)

	var target reflect.Value
	var typed reflect.Value
	haveTyped := false

	if t := d.factory.GetType(uint32(rawID)); t != nil {
		typed = reflect.New(t).Elem()
		target = typed
		haveTyped = true
	} else if field.IsValid() && field.Kind() == reflect.Interface && field.NumMethod() == 0 && isPrimitiveBase(base) {
		target = field
	}

	if err := d.readValue(r, base, target); err != nil {
		return err
	}

	if haveTyped && field.IsValid() && field.Kind() == reflect.Interface {
		if !typed.Type().AssignableTo(field.Type()) {
			return typeMismatch(field, base)
		}
		field.Set(typed)
	}

	term, err := r.readByte()
	if err != nil {
		return err
	}
	if term != 0x00 {
		return errors.New("tdf: missing variable terminator")
	}
	return nil
}

func writeLegacyBaseTypeAndSize(buf *bytes.Buffer, t TdfLegacyBaseType, size int) error {
	return ErrLegacyWireNotPorted
}

func readLegacyBaseTypeAndSize(r *tdfReader) (TdfLegacyBaseType, byte, error) {
	return 0, 0, ErrLegacyWireNotPorted
}

func writeLegacyInteger(buf *bytes.Buffer, n int) error {
	return ErrLegacyWireNotPorted
}

func readLegacyInteger(r *tdfReader) (uint64, error) {
	return 0, ErrLegacyWireNotPorted
}

func readLegacyIntegerSized(r *tdfReader, size byte) (uint64, error) {
	return 0, ErrLegacyWireNotPorted
}

func writeLegacyString(buf *bytes.Buffer, s string, withType bool) error {
	return ErrLegacyWireNotPorted
}

func readLegacyString(r *tdfReader, size byte) (string, error) {
	return "", ErrLegacyWireNotPorted
}

func writeLegacyBlob(buf *bytes.Buffer, b []byte, withType bool) error {
	return ErrLegacyWireNotPorted
}

func readLegacyBlob(r *tdfReader, size byte) ([]byte, error) {
	return nil, ErrLegacyWireNotPorted
}

func legacyBaseTypeOf(t reflect.Type) (TdfLegacyBaseType, error) {
	t = derefType(t)

	switch t.Kind() {
	case reflect.Bool, reflect.Int8:
		return LegacyTypeInt8, nil
	case reflect.Uint8:
		return LegacyTypeUInt8, nil
	case reflect.Int16:
		return LegacyTypeInt16, nil
	case reflect.Uint16:
		return LegacyTypeUInt16, nil
	case reflect.Int32:
		return LegacyTypeInt32, nil
	case reflect.Uint32:
		return LegacyTypeUInt32, nil
	case reflect.Int64, reflect.Int:
		return LegacyTypeInt64, nil
	case reflect.Uint64, reflect.Uint:
		return LegacyTypeUInt64, nil
	case reflect.String:
		return LegacyTypeString, nil
	case reflect.Slice:
		if t.Elem() == byteType {
			return LegacyTypeBlob, nil
		}
		return LegacyTypeArray, nil
	case reflect.Map:
		return LegacyTypeMap, nil
	case reflect.Struct:
		if t == timeValueType || t == blazeObjectTypeType || t == blazeObjectIDType {
			break
		}
		if isUnionType(t) {
			return LegacyTypeUnion, nil
		}
		return LegacyTypeStruct, nil
	}

	return 0, fmt.Errorf("tdf: unknown legacy base type for %s", t)
}

func legacyDefaultTypeSize(t TdfLegacyBaseType) int {
	switch t {
	case LegacyTypeString, LegacyTypeBlob, LegacyTypeMap:
		return 15
	case LegacyTypeInt8, LegacyTypeUInt8:
		return 1
	case LegacyTypeInt16, LegacyTypeUInt16:
		return 2
	case LegacyTypeInt32, LegacyTypeUInt32:
		return 4
	case LegacyTypeInt64, LegacyTypeUInt64:
		return 8
	case LegacyTypeArray:
		return 1
	}
	return 0 
}

func legacyIntInfo(t TdfLegacyBaseType) (width int, signed bool, ok bool) {
	switch t {
	case LegacyTypeInt8:
		return 1, true, true
	case LegacyTypeUInt8:
		return 1, false, true
	case LegacyTypeInt16:
		return 2, true, true
	case LegacyTypeUInt16:
		return 2, false, true
	case LegacyTypeInt32:
		return 4, true, true
	case LegacyTypeUInt32:
		return 4, false, true
	case LegacyTypeInt64:
		return 8, true, true
	case LegacyTypeUInt64:
		return 8, false, true
	}
	return 0, false, false
}

func legacyIntWidth(t TdfLegacyBaseType) (int, bool) {
	w, _, ok := legacyIntInfo(t)
	return w, ok
}

func writeFixedBE(buf *bytes.Buffer, width int, bits uint64) {
	for i := width - 1; i >= 0; i-- {
		buf.WriteByte(byte(bits >> (8 * uint(i))))
	}
}

func setLegacyInteger(field reflect.Value, bits uint64, width int, signed bool) error {
	var i int64
	var u uint64

	if signed {
		shift := uint(64 - 8*width)
		i = int64(bits<<shift) >> shift
	} else {
		u = bits
	}

	switch field.Kind() {
	case reflect.Bool:
		field.SetBool(bits != 0)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if !signed {
			if u > math.MaxInt64 {
				return fmt.Errorf("tdf: value %d overflows %s", u, field.Type())
			}
			i = int64(u)
		}
		if field.OverflowInt(i) {
			return fmt.Errorf("tdf: value %d overflows %s", i, field.Type())
		}
		field.SetInt(i)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if signed {
			if i < 0 {
				return fmt.Errorf("tdf: value %d overflows %s", i, field.Type())
			}
			u = uint64(i)
		}
		if field.OverflowUint(u) {
			return fmt.Errorf("tdf: value %d overflows %s", u, field.Type())
		}
		field.SetUint(u)

	default:
		return fmt.Errorf("tdf: cannot decode legacy integer into Go type %s", field.Type())
	}
	return nil
}

type TdfLegacyEncoder struct {
	factory *TdfFactory
}

func (e *TdfLegacyEncoder) Encode(obj interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.encodeInto(&buf, obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (e *TdfLegacyEncoder) WriteTo(w io.Writer, obj interface{}) error {
	var buf bytes.Buffer
	if err := e.encodeInto(&buf, obj); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func (e *TdfLegacyEncoder) encodeInto(buf *bytes.Buffer, obj interface{}) error {
	v, ok := indirect(reflect.ValueOf(obj))
	if !ok || v.Kind() != reflect.Struct {
		return errors.New("tdf: Encode needs a struct or a non-nil pointer to a struct")
	}
	return e.writeStructBody(buf, v)
}

func (e *TdfLegacyEncoder) writeStructBody(buf *bytes.Buffer, v reflect.Value) error {
	info, err := getStructInfo(v.Type())
	if err != nil {
		return err
	}

	for _, f := range info.declared {
		fv := v.Field(f.index)
		if isNilValue(fv) {
			continue
		}

		t := derefType(fv.Type())
		base, err := legacyBaseTypeOf(t)
		if err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}

		val, ok := indirect(fv)
		if !ok {
			continue
		}

		if (base == LegacyTypeArray || base == LegacyTypeMap) && val.Len() == 0 {
			continue
		}

		buf.Write(f.member.Bytes[:])

		if err := e.writeValue(buf, t, base, val, false); err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
	}

	return nil
}

func (e *TdfLegacyEncoder) writeValue(buf *bytes.Buffer, t reflect.Type, base TdfLegacyBaseType, v reflect.Value, withoutType bool) error {
	if width, ok := legacyIntWidth(base); ok {
		if !withoutType {
			if err := writeLegacyBaseTypeAndSize(buf, base, width); err != nil {
				return err
			}
		}

		var bits uint64
		switch v.Kind() {
		case reflect.Bool:
			if v.Bool() {
				bits = 1
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			bits = uint64(v.Int())
		default:
			bits = v.Uint()
		}

		writeFixedBE(buf, width, bits)
		return nil
	}

	switch base {
	case LegacyTypeStruct:
		if !withoutType {
			if err := writeLegacyBaseTypeAndSize(buf, LegacyTypeStruct, 0); err != nil {
				return err
			}
		}
		return e.writeStruct(buf, v)

	case LegacyTypeString:
		return writeLegacyString(buf, v.String(), !withoutType)

	case LegacyTypeBlob:
		return writeLegacyBlob(buf, v.Bytes(), !withoutType)

	case LegacyTypeArray:
		return e.writeArray(buf, t, v, withoutType)

	case LegacyTypeMap:
		return e.writeMap(buf, t, v)

	case LegacyTypeUnion:
		return e.writeUnion(buf, v, withoutType)
	}

	return fmt.Errorf("tdf: no legacy writer for %s", base)
}

func (e *TdfLegacyEncoder) writeStruct(buf *bytes.Buffer, v reflect.Value) error {
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("tdf: %s is not a struct", v.Type())
	}
	if err := e.writeStructBody(buf, v); err != nil {
		return err
	}
	buf.WriteByte(0x00) 
	return nil
}

func (e *TdfLegacyEncoder) writeArray(buf *bytes.Buffer, t reflect.Type, v reflect.Value, withoutType bool) error {
	et := derefType(t.Elem())
	eb, err := legacyBaseTypeOf(et)
	if err != nil {
		return fmt.Errorf("tdf: list type %s not supported: %w", et, err)
	}

	if !withoutType {
		if err := writeLegacyBaseTypeAndSize(buf, LegacyTypeArray, 1); err != nil {
			return err
		}
	}

	if err := writeLegacyInteger(buf, v.Len()); err != nil {
		return err
	}
	if err := writeLegacyBaseTypeAndSize(buf, eb, legacyDefaultTypeSize(eb)); err != nil {
		return err
	}

	for i := 0; i < v.Len(); i++ {
		item, ok := indirect(v.Index(i))
		if !ok {
			return fmt.Errorf("tdf: nil element at array index %d", i)
		}
		if err := e.writeValue(buf, et, eb, item, true); err != nil {
			return err
		}
	}
	return nil
}

func (e *TdfLegacyEncoder) writeMap(buf *bytes.Buffer, t reflect.Type, v reflect.Value) error {
	if v.Len() == 0 {
		return errors.New("tdf: legacy encoder cannot write an empty nested map")
	}

	kt := derefType(t.Key())
	vt := derefType(t.Elem())

	kb, err := legacyBaseTypeOf(kt)
	if err != nil {
		return fmt.Errorf("tdf: map key type %s not supported: %w", kt, err)
	}
	vb, err := legacyBaseTypeOf(vt)
	if err != nil {
		return fmt.Errorf("tdf: map value type %s not supported: %w", vt, err)
	}

	keys := v.MapKeys()
	sortMapKeys(keys)

	if err := writeLegacyBaseTypeAndSize(buf, LegacyTypeMap, len(keys)); err != nil {
		return err
	}

	for i, k := range keys {
		kv, ok := indirect(k)
		if !ok {
			return errors.New("tdf: nil map key")
		}

		if i == 0 {
			if err := writeLegacyBaseTypeAndSize(buf, kb, legacyDefaultTypeSize(kb)); err != nil {
				return err
			}
		}
		if err := e.writeValue(buf, kt, kb, kv, true); err != nil {
			return err
		}

		vv, ok := indirect(v.MapIndex(k))
		if !ok {
			return errors.New("tdf: nil map value")
		}

		if i == 0 {
			if err := writeLegacyBaseTypeAndSize(buf, vb, legacyDefaultTypeSize(vb)); err != nil {
				return err
			}
		}
		if err := e.writeValue(buf, vt, vb, vv, true); err != nil {
			return err
		}
	}
	return nil
}

func (e *TdfLegacyEncoder) writeUnion(buf *bytes.Buffer, v reflect.Value, withoutType bool) error {
	info, err := getUnionInfo(v.Type())
	if err != nil {
		return err
	}

	active, member, ok := unionCurrent(v, info)
	if !ok {
		active = UnionUnsetMember
	}

	if !withoutType {
		if err := writeLegacyBaseTypeAndSize(buf, LegacyTypeUnion, 0); err != nil {
			return err
		}
	}

	buf.WriteByte(active)

	if active != UnionUnsetMember {
		buf.Write(tdfLegacyUnionValuTag)
		if err := writeLegacyBaseTypeAndSize(buf, LegacyTypeStruct, 0); err != nil {
			return err
		}
		return e.writeStruct(buf, member)
	}
	return nil
}

type TdfLegacyDecoder struct {
	factory *TdfFactory
}

func (d *TdfLegacyDecoder) Decode(data []byte, out interface{}) error {
	target, info, err := prepareDecodeTarget(out)
	if err != nil {
		return err
	}

	r := &tdfReader{data: data}
	for r.remaining() > 0 {
		if err := d.readTdf(r, target, info); err != nil {
			return err
		}
	}
	return nil
}

func (d *TdfLegacyDecoder) DecodeFrom(r io.Reader, out interface{}) error {
	data, err := readAll(r)
	if err != nil {
		return err
	}
	return d.Decode(data, out)
}

func (d *TdfLegacyDecoder) readTdf(r *tdfReader, target reflect.Value, info *structInfo) error {
	tagBytes, err := r.readBytes(TdfTagLength)
	if err != nil {
		return err
	}

	var tb [TdfTagLength]byte
	copy(tb[:], tagBytes)
	member := TdfMemberFromBytes(tb)

	base, size, err := readLegacyBaseTypeAndSize(r)
	if err != nil {
		return err
	}

	var field reflect.Value
	if idx, ok := info.byTag[member.Tag]; ok {
		field = target.Field(idx)
	}

	if err := d.readValue(r, base, size, field); err != nil {
		return fmt.Errorf("tag %q: %w", member.Tag, err)
	}
	return nil
}

func (d *TdfLegacyDecoder) readValue(r *tdfReader, base TdfLegacyBaseType, size byte, field reflect.Value) error {
	if field.IsValid() {
		switch field.Kind() {
		case reflect.Ptr:
			nv := reflect.New(field.Type().Elem())
			if err := d.readValue(r, base, size, nv.Elem()); err != nil {
				return err
			}
			field.Set(nv)
			return nil
		case reflect.Interface:
			field = reflect.Value{} 
		}
	}

	if width, signed, ok := legacyIntInfo(base); ok {
		return d.readInt(r, size, width, signed, field)
	}

	switch base {
	case LegacyTypeStruct:
		return d.readStruct(r, field)
	case LegacyTypeString:
		return d.readString(r, size, field)
	case LegacyTypeArray:
		return d.readArray(r, field)
	case LegacyTypeBlob:
		return d.readBlob(r, size, field)
	case LegacyTypeMap:
		return d.readMap(r, size, field)
	case LegacyTypeUnion:
		return d.readUnion(r, field)
	}

	return fmt.Errorf("tdf: %s is not supported", base)
}

func (d *TdfLegacyDecoder) readInt(r *tdfReader, size byte, width int, signed bool, field reflect.Value) error {
	if int(size) != width {
		return fmt.Errorf("tdf: legacy integer size %d does not match width %d", size, width)
	}

	b, err := r.readBytes(width)
	if err != nil {
		return err
	}

	var bits uint64
	for _, x := range b {
		bits = bits<<8 | uint64(x)
	}

	if !field.IsValid() {
		return nil
	}
	return setLegacyInteger(field, bits, width, signed)
}

func (d *TdfLegacyDecoder) readString(r *tdfReader, size byte, field reflect.Value) error {
	s, err := readLegacyString(r, size)
	if err != nil {
		return err
	}

	if !field.IsValid() {
		return nil
	}

	if field.Kind() != reflect.String {
		return fmt.Errorf("tdf: cannot decode legacy string into Go type %s", field.Type())
	}
	field.SetString(s)
	return nil
}

func (d *TdfLegacyDecoder) readBlob(r *tdfReader, size byte, field reflect.Value) error {
	b, err := readLegacyBlob(r, size)
	if err != nil {
		return err
	}

	if !field.IsValid() {
		return nil
	}

	if field.Kind() != reflect.Slice || field.Type().Elem() != byteType {
		return fmt.Errorf("tdf: cannot decode legacy blob into Go type %s", field.Type())
	}
	field.SetBytes(append([]byte(nil), b...))
	return nil
}

func (d *TdfLegacyDecoder) readStructBody(r *tdfReader, sv reflect.Value, info *structInfo) error {
	for {
		b, err := r.readByte()
		if err != nil {
			return err
		}

		if b == 0x00 {
			return nil
		}

		r.pos--

		if err := d.readTdf(r, sv, info); err != nil {
			return err
		}
	}
}

func (d *TdfLegacyDecoder) readStruct(r *tdfReader, field reflect.Value) error {
	if !field.IsValid() {
		return d.readStructBody(r, reflect.Value{}, emptyStructInfo)
	}

	if field.Kind() != reflect.Struct {
		return fmt.Errorf("tdf: cannot decode legacy struct into Go type %s", field.Type())
	}

	info, err := getStructInfo(field.Type())
	if err != nil {
		return err
	}

	field.Set(reflect.Zero(field.Type()))
	return d.readStructBody(r, field, info)
}

func (d *TdfLegacyDecoder) readArray(r *tdfReader, field reflect.Value) error {
	count, err := readLegacyInteger(r)
	if err != nil {
		return err
	}

	base, size, err := readLegacyBaseTypeAndSize(r)
	if err != nil {
		return err
	}

	if count > uint64(r.remaining()) {
		return io.ErrUnexpectedEOF
	}

	if !field.IsValid() { 
		for i := uint64(0); i < count; i++ {
			if err := d.readValue(r, base, size, reflect.Value{}); err != nil {
				return err
			}
		}
		return nil
	}

	if field.Kind() != reflect.Slice || field.Type().Elem() == byteType {
		return fmt.Errorf("tdf: cannot decode legacy array into Go type %s", field.Type())
	}

	elemType := field.Type().Elem()
	list := reflect.MakeSlice(field.Type(), 0, int(count))

	for i := uint64(0); i < count; i++ {
		elem := reflect.New(elemType).Elem()
		if err := d.readValue(r, base, size, elem); err != nil {
			return err
		}
		list = reflect.Append(list, elem)
	}

	field.Set(list)
	return nil
}

func (d *TdfLegacyDecoder) readMap(r *tdfReader, size byte, field reflect.Value) error {
	count, err := readLegacyIntegerSized(r, size)
	if err != nil {
		return err
	}

	if count == 0 {
		if field.IsValid() && field.Kind() == reflect.Map {
			field.Set(reflect.MakeMap(field.Type()))
		}
		return nil
	}

	if count > uint64(r.remaining()) {
		return io.ErrUnexpectedEOF
	}

	var mapType reflect.Type
	var m reflect.Value
	if field.IsValid() {
		if field.Kind() != reflect.Map {
			return fmt.Errorf("tdf: cannot decode legacy map into Go type %s", field.Type())
		}
		mapType = field.Type()
		m = reflect.MakeMapWithSize(mapType, int(count))
	}

	var keyBase, valueBase TdfLegacyBaseType
	var keySize, valueSize byte

	for i := uint64(0); i < count; i++ {
		if i == 0 {
			keyBase, keySize, err = readLegacyBaseTypeAndSize(r)
			if err != nil {
				return err
			}
		}

		var k reflect.Value
		if field.IsValid() {
			k = reflect.New(mapType.Key()).Elem()
		}
		if err := d.readValue(r, keyBase, keySize, k); err != nil {
			return err
		}

		if i == 0 {
			valueBase, valueSize, err = readLegacyBaseTypeAndSize(r)
			if err != nil {
				return err
			}
		}

		var v reflect.Value
		if field.IsValid() {
			v = reflect.New(mapType.Elem()).Elem()
		}
		if err := d.readValue(r, valueBase, valueSize, v); err != nil {
			return err
		}

		if field.IsValid() {
			m.SetMapIndex(k, v)
		}
	}

	if field.IsValid() {
		field.Set(m)
	}
	return nil
}

func (d *TdfLegacyDecoder) readUnion(r *tdfReader, field reflect.Value) error {
	am, err := r.readByte()
	if err != nil {
		return err
	}

	var info *unionInfo
	if field.IsValid() {
		if field.Kind() != reflect.Struct || !isUnionType(field.Type()) {
			return fmt.Errorf("tdf: cannot decode legacy union into Go type %s", field.Type())
		}

		info, err = getUnionInfo(field.Type())
		if err != nil {
			return err
		}

		field.Set(reflect.Zero(field.Type()))
		unionSetActive(field, info, UnionUnsetMember)
	}

	if am == UnionUnsetMember {
		return nil
	}

	if r.remaining() >= len(tdfLegacyUnionValuTag) &&
		bytes.Equal(r.data[r.pos:r.pos+len(tdfLegacyUnionValuTag)], tdfLegacyUnionValuTag) {
		save := r.pos
		r.pos += len(tdfLegacyUnionValuTag)

		base, _, err := readLegacyBaseTypeAndSize(r)
		if err != nil {
			return err
		}
		if base != LegacyTypeStruct {
			r.pos = save
		}
	}

	if !field.IsValid() {
		return d.readStructBody(r, reflect.Value{}, emptyStructInfo)
	}

	idx, ok := info.byActive[am]
	if !ok {
		return d.readStructBody(r, reflect.Value{}, emptyStructInfo)
	}

	m := info.members[idx]
	sinfo, err := getStructInfo(m.structType)
	if err != nil {
		return err
	}

	nv := reflect.New(m.structType)
	if err := d.readStructBody(r, nv.Elem(), sinfo); err != nil {
		return err
	}

	field.Field(m.index).Set(nv)
	unionSetActive(field, info, am)
	return nil
}

func IPToUInt(ip string) uint32 {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return 0
	}

	ip4 := parsed.To4()
	if ip4 == nil {
		return 0
	}

	return binary.BigEndian.Uint32(ip4)
}
