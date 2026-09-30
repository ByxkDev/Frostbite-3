package blaze

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"strings"

	"bf4/logger"
)

const (
	TDF_INT8          byte = 0x00
	TDF_INT16         byte = 0x01
	TDF_INT32         byte = 0x02
	TDF_INT64         byte = 0x03
	TDF_UINT8         byte = 0x04
	TDF_UINT16        byte = 0x05
	TDF_UINT32        byte = 0x06
	TDF_UINT64        byte = 0x07
	TDF_STRING        byte = 0x08
	TDF_BLOB          byte = 0x09
	TDF_STRUCT        byte = 0x0A
	TDF_LIST          byte = 0x0B
	TDF_MAP           byte = 0x0C
	TDF_UNION         byte = 0x0D
	TDF_VARIABLE      byte = 0x0E
	TDF_GENERIC_TYPE  byte = 0x0F
	TDF_GENERIC_TYPE2 byte = 0x10
	TDF_FLOAT         byte = 0x11
	TDF_TIME          byte = 0x12
	TDF_BOOL          byte = 0x13
)

const (
	wireInt     byte = 0x00
	wireString  byte = 0x01
	wireBlob    byte = 0x02
	wireStruct  byte = 0x03
	wireList    byte = 0x04
	wireMap     byte = 0x05
	wireUnion   byte = 0x06
	wireUnknown byte = 0x07
	wireVector  byte = 0x08
	wireVector3 byte = 0x09
)

type TDF struct {
	Tag   string
	Type  byte
	Value interface{}
}

type TDFUnion struct {
	ActiveMember byte
	Data         []byte
}

func tdfWireType(t byte) byte {
	switch t {
	case TDF_INT8, TDF_INT16, TDF_INT32, TDF_INT64,
		TDF_UINT8, TDF_UINT16, TDF_UINT32, TDF_UINT64,
		TDF_BOOL:
		return wireInt
	case TDF_STRING:
		return wireString
	case TDF_BLOB:
		return wireBlob
	case TDF_STRUCT:
		return wireStruct
	case TDF_LIST:
		return wireList
	case TDF_MAP:
		return wireMap
	case TDF_UNION:
		return wireUnion
	case TDF_VARIABLE:
		return wireUnknown
	case TDF_FLOAT:
		return wireVector
	case TDF_TIME:
		return wireVector3
	default:
		return t
	}
}

func WriteTag(buf *bytes.Buffer, tag string) {
	tag = strings.ToUpper(tag)

	if len(tag) == 0 || len(tag) > 4 {
		return
	}

	var output [3]byte
	raw := []byte(tag)

	if len(raw) > 0 {
		output[0] |= (raw[0] & 0x40) << 1
		output[0] |= (raw[0] & 0x10) << 2
		output[0] |= (raw[0] & 0x0F) << 2
	}

	if len(raw) > 1 {
		output[0] |= (raw[1] & 0x40) >> 5
		output[0] |= (raw[1] & 0x10) >> 4
		output[1] |= (raw[1] & 0x0F) << 4
	}

	if len(raw) > 2 {
		output[1] |= (raw[2] & 0x40) >> 3
		output[1] |= (raw[2] & 0x10) >> 2
		output[1] |= (raw[2] & 0x0C) >> 2
		output[2] |= (raw[2] & 0x03) << 6
	}

	if len(raw) > 3 {
		output[2] |= (raw[3] & 0x40) >> 1
		output[2] |= raw[3] & 0x1F
	}

	buf.Write(output[:])
}

func ReadTag(data []byte, offset int) (string, int) {
	if offset+3 > len(data) {
		return "", -1
	}

	input := data[offset : offset+3]

	decode := func(m byte, c byte) byte {
		if m|c == 0x00 {
			return 0
		}

		if m&0x40 == 0 {
			return 0x30 | c
		}

		return m | c
	}

	var output [4]byte

	output[0] = decode(
		(input[0]&0x80)>>1,
		(input[0]&0x7C)>>2,
	)

	output[1] = decode(
		(input[0]&0x02)<<5,
		((input[0]&0x01)<<4)|((input[1]&0xF0)>>4),
	)

	output[2] = decode(
		(input[1]&0x08)<<3,
		((input[1]&0x07)<<2)|((input[2]&0xC0)>>6),
	)

	output[3] = decode(
		(input[2]&0x20)<<1,
		input[2]&0x1F,
	)

	length := 4

	for length > 0 && output[length-1] == 0 {
		length--
	}

	return string(output[:length]), offset + 3
}

func WriteTDF(buf *bytes.Buffer, tag string, value string) {
	WriteTag(buf, tag)
	buf.WriteByte(wireString)
	WriteTDFStringValue(buf, value)
}

func WriteTDFStringValue(buf *bytes.Buffer, value string) {
	data := []byte(value)

	WriteTDFInteger(buf, int64(len(data)+1))
	buf.Write(data)
	buf.WriteByte(0)
}

func WriteTDFInteger(buf *bytes.Buffer, value int64) {
	if value == 0 {
		buf.WriteByte(0)
		return
	}

	negative := value < 0

	var magnitude uint64

	if negative {
		if value == math.MinInt64 {
			magnitude = uint64(math.MaxInt64) + 1
		} else {
			magnitude = uint64(-value)
		}
	} else {
		magnitude = uint64(value)
	}

	first := byte(magnitude & 0x3F)

	if negative {
		first |= 0x40
	}

	magnitude >>= 6

	if magnitude != 0 {
		first |= 0x80
	}

	buf.WriteByte(first)

	for magnitude != 0 {
		next := byte(magnitude & 0x7F)
		magnitude >>= 7

		if magnitude != 0 {
			next |= 0x80
		}

		buf.WriteByte(next)
	}
}

func ReadTDFInteger(data []byte, offset int) (int64, int) {
	if offset >= len(data) {
		return 0, -1
	}

	first := data[offset]
	offset++

	negative := first&0x40 != 0
	continuation := first&0x80 != 0

	var value uint64 = uint64(first & 0x3F)
	shift := uint(6)

	for continuation {
		if offset >= len(data) {
			return 0, -1
		}

		b := data[offset]
		offset++

		value |= uint64(b&0x7F) << shift
		shift += 7

		if shift > 63 && b&0x80 != 0 {
			return 0, -1
		}

		continuation = b&0x80 != 0
	}

	if negative {
		if value == uint64(math.MaxInt64)+1 {
			return math.MinInt64, offset
		}

		if value > uint64(math.MaxInt64) {
			return 0, -1
		}

		return -int64(value), offset
	}

	if value > uint64(math.MaxInt64) {
		return 0, -1
	}

	return int64(value), offset
}

func WriteInt8(buf *bytes.Buffer, tag string, value int8) {
	WriteTag(buf, tag)
	buf.WriteByte(wireInt)
	WriteTDFInteger(buf, int64(value))
}

func WriteInt16(buf *bytes.Buffer, tag string, value int16) {
	WriteTag(buf, tag)
	buf.WriteByte(wireInt)
	WriteTDFInteger(buf, int64(value))
}

func WriteInt32(buf *bytes.Buffer, tag string, value int32) {
	WriteTag(buf, tag)
	buf.WriteByte(wireInt)
	WriteTDFInteger(buf, int64(value))
}

func WriteInt64(buf *bytes.Buffer, tag string, value int64) {
	WriteTag(buf, tag)
	buf.WriteByte(wireInt)
	WriteTDFInteger(buf, value)
}

func WriteUInt8(buf *bytes.Buffer, tag string, value uint8) {
	WriteTag(buf, tag)
	buf.WriteByte(wireInt)
	WriteTDFInteger(buf, int64(value))
}

func WriteUInt16(buf *bytes.Buffer, tag string, value uint16) {
	WriteTag(buf, tag)
	buf.WriteByte(wireInt)
	WriteTDFInteger(buf, int64(value))
}

func WriteUInt32(buf *bytes.Buffer, tag string, value uint32) {
	WriteTag(buf, tag)
	buf.WriteByte(wireInt)
	WriteTDFInteger(buf, int64(value))
}

func WriteUInt64(buf *bytes.Buffer, tag string, value uint64) {
	WriteTag(buf, tag)
	buf.WriteByte(wireInt)

	if value > math.MaxInt64 {
		var magnitude uint64 = value
		first := byte(magnitude & 0x3F)
		magnitude >>= 6

		if magnitude != 0 {
			first |= 0x80
		}

		buf.WriteByte(first)

		for magnitude != 0 {
			next := byte(magnitude & 0x7F)
			magnitude >>= 7

			if magnitude != 0 {
				next |= 0x80
			}

			buf.WriteByte(next)
		}

		return
	}

	WriteTDFInteger(buf, int64(value))
}

func WriteBool(buf *bytes.Buffer, tag string, value bool) {
	WriteTag(buf, tag)
	buf.WriteByte(wireInt)

	if value {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
}

func WriteBlob(buf *bytes.Buffer, tag string, value []byte) {
	WriteTag(buf, tag)
	buf.WriteByte(wireBlob)
	WriteTDFInteger(buf, int64(len(value)))
	buf.Write(value)
}

func WriteStruct(buf *bytes.Buffer, tag string, data []byte) {
	WriteTag(buf, tag)
	buf.WriteByte(wireStruct)
	buf.Write(data)
	buf.WriteByte(0)
}

func WriteList(buf *bytes.Buffer, tag string, elementType byte, elements [][]byte) {
	WriteTag(buf, tag)
	buf.WriteByte(wireList)
	buf.WriteByte(tdfWireType(elementType))
	WriteTDFInteger(buf, int64(len(elements)))

	for _, element := range elements {
		writeRawTDFValue(buf, tdfWireType(elementType), element)
	}
}

func WriteStringList(buf *bytes.Buffer, tag string, values []string) {
	WriteTag(buf, tag)
	buf.WriteByte(wireList)
	buf.WriteByte(wireString)
	WriteTDFInteger(buf, int64(len(values)))

	for _, value := range values {
		WriteTDFStringValue(buf, value)
	}
}

func WriteStructList(buf *bytes.Buffer, tag string, elements [][]byte) {
	WriteTag(buf, tag)
	buf.WriteByte(wireList)
	buf.WriteByte(wireStruct)
	WriteTDFInteger(buf, int64(len(elements)))

	for _, element := range elements {
		buf.Write(element)
		buf.WriteByte(0)
	}
}

func WriteMap(buf *bytes.Buffer, tag string, keyType byte, valueType byte, values [][]byte) {
	WriteTag(buf, tag)
	buf.WriteByte(wireMap)

	keyWireType := tdfWireType(keyType)
	valueWireType := tdfWireType(valueType)

	buf.WriteByte(keyWireType)
	buf.WriteByte(valueWireType)

	count := len(values) / 2
	WriteTDFInteger(buf, int64(count))

	for i := 0; i+1 < len(values); i += 2 {
		writeRawTDFValue(buf, keyWireType, values[i])
		writeRawTDFValue(buf, valueWireType, values[i+1])
	}
}

func WriteStringMap(buf *bytes.Buffer, tag string, values map[string]string, order []string) {
	WriteTag(buf, tag)
	buf.WriteByte(wireMap)
	buf.WriteByte(wireString)
	buf.WriteByte(wireString)
	WriteTDFInteger(buf, int64(len(order)))

	for _, key := range order {
		value, ok := values[key]
		if !ok {
			continue
		}

		WriteTDFStringValue(buf, key)
		WriteTDFStringValue(buf, value)
	}
}

func writeRawTDFValue(buf *bytes.Buffer, wireType byte, value []byte) {
	switch wireType {
	case wireString:
		WriteTDFStringValue(buf, string(value))

	case wireBlob:
		WriteTDFInteger(buf, int64(len(value)))
		buf.Write(value)

	case wireStruct:
		buf.Write(value)
		buf.WriteByte(0)

	default:
		buf.Write(value)
	}
}

func WriteUnion(buf *bytes.Buffer, tag string, activeMember byte, value []byte) {
	WriteTag(buf, tag)
	buf.WriteByte(wireUnion)
	buf.WriteByte(activeMember)

	if activeMember != 0x7F {
		WriteTag(buf, "VALU")
		buf.Write(value)
		buf.WriteByte(0)
	}
}

func EncodeServerAddress(ip string, port uint32) []byte {
	var buf bytes.Buffer

	WriteTag(&buf, "ADDR")
	buf.WriteByte(wireUnion)
	buf.WriteByte(0x00)

	WriteTag(&buf, "VALU")

	WriteTag(&buf, "HOST")
	buf.WriteByte(wireString)
	WriteTDFStringValue(&buf, ip)

	WriteTag(&buf, "IP")
	buf.WriteByte(wireInt)
	WriteTDFInteger(&buf, int64(IPToUInt(ip)))

	WriteTag(&buf, "PORT")
	buf.WriteByte(wireInt)
	WriteTDFInteger(&buf, int64(port))

	buf.WriteByte(0)

	return buf.Bytes()
}

func EncodeServerInstanceInfo(ip string, port uint32, messages []string, secure bool) []byte {
	var buf bytes.Buffer

	WriteTag(&buf, "ADDR")
	buf.WriteByte(wireUnion)
	buf.WriteByte(0x00)

	WriteTag(&buf, "VALU")

	WriteTag(&buf, "HOST")
	buf.WriteByte(wireString)
	WriteTDFStringValue(&buf, ip)

	WriteTag(&buf, "IP")
	buf.WriteByte(wireInt)
	WriteTDFInteger(&buf, int64(IPToUInt(ip)))

	WriteTag(&buf, "PORT")
	buf.WriteByte(wireInt)
	WriteTDFInteger(&buf, int64(port))

	buf.WriteByte(0)

	WriteTag(&buf, "AMAP")
	buf.WriteByte(wireList)
	buf.WriteByte(wireStruct)
	WriteTDFInteger(&buf, 0)

	WriteTag(&buf, "MSGS")
	buf.WriteByte(wireList)
	buf.WriteByte(wireString)
	WriteTDFInteger(&buf, int64(len(messages)))

	for _, message := range messages {
		WriteTDFStringValue(&buf, message)
	}

	WriteTag(&buf, "NMAP")
	buf.WriteByte(wireList)
	buf.WriteByte(wireStruct)
	WriteTDFInteger(&buf, 0)

	WriteTag(&buf, "SECU")
	buf.WriteByte(wireInt)
	WriteTDFInteger(&buf, 0)

	if secure {
		buf.Bytes()[buf.Len()-1] = 1
	}

	WriteTag(&buf, "XDNS")
	buf.WriteByte(wireInt)
	WriteTDFInteger(&buf, 0)

	return buf.Bytes()
}

func ReadTDF(data []byte) []TDF {
	var result []TDF
	offset := 0

	for offset < len(data) {
		if data[offset] == 0x00 {
			offset++
			continue
		}

		tag, next := ReadTag(data, offset)
		if next < 0 || tag == "" {
			return result
		}

		offset = next

		if offset >= len(data) {
			return result
		}

		wireType := data[offset]
		offset++

		value, next := readTDFValue(data, offset, wireType)
		if next < 0 {
			return result
		}

		offset = next

		result = append(result, TDF{
			Tag:   tag,
			Type:  wireType,
			Value: value,
		})
	}

	return result
}

func readTDFValue(data []byte, offset int, wireType byte) (interface{}, int) {
	switch wireType {
	case wireInt:
		value, next := ReadTDFInteger(data, offset)
		if next < 0 {
			return nil, -1
		}

		return value, next

	case wireString:
		value, next := ReadTDFStringValue(data, offset)
		if next < 0 {
			return nil, -1
		}

		return value, next

	case wireBlob:
		length, next := ReadTDFInteger(data, offset)
		if next < 0 || length < 0 || int64(len(data)-next) < length {
			return nil, -1
		}

		value := append([]byte(nil), data[next:next+int(length)]...)
		return value, next + int(length)

	case wireStruct:
		start := offset
		next := skipTDFStruct(data, offset)

		if next < 0 || next > len(data) {
			return nil, -1
		}

		value := append([]byte(nil), data[start:next-1]...)
		return value, next

	case wireList:
		if offset >= len(data) {
			return nil, -1
		}

		elementType := data[offset]
		offset++

		count, next := ReadTDFInteger(data, offset)
		if next < 0 || count < 0 {
			return nil, -1
		}

		offset = next

		elements := make([][]byte, 0, int(count))

		for i := int64(0); i < count; i++ {
			start := offset

			next = skipTDFValue(data, offset, elementType)
			if next < 0 || next > len(data) {
				return nil, -1
			}

			elements = append(elements, append([]byte(nil), data[start:next]...))
			offset = next
		}

		return elements, offset

	case wireMap:
		if offset+2 > len(data) {
			return nil, -1
		}

		keyType := data[offset]
		valueType := data[offset+1]
		offset += 2

		count, next := ReadTDFInteger(data, offset)
		if next < 0 || count < 0 {
			return nil, -1
		}

		offset = next

		entries := make([][]byte, 0, int(count)*2)

		for i := int64(0); i < count; i++ {
			keyStart := offset

			offset = skipTDFValue(data, offset, keyType)
			if offset < 0 || offset > len(data) {
				return nil, -1
			}

			entries = append(entries, append([]byte(nil), data[keyStart:offset]...))

			valueStart := offset

			offset = skipTDFValue(data, offset, valueType)
			if offset < 0 || offset > len(data) {
				return nil, -1
			}

			entries = append(entries, append([]byte(nil), data[valueStart:offset]...))
		}

		return entries, offset

	case wireUnion:
		if offset >= len(data) {
			return nil, -1
		}

		activeMember := data[offset]
		offset++

		if activeMember == 0x7F {
			return TDFUnion{
				ActiveMember: activeMember,
				Data:         nil,
			}, offset
		}

		unionTag, next := ReadTag(data, offset)
		if next < 0 || unionTag != "VALU" {
			return nil, -1
		}

		offset = next

		start := offset
		offset = skipTDFStruct(data, offset)

		if offset < 0 || offset > len(data) {
			return nil, -1
		}

		return TDFUnion{
			ActiveMember: activeMember,
			Data:         append([]byte(nil), data[start:offset-1]...),
		}, offset

	case wireUnknown, wireVector, wireVector3:
		return nil, -1

	default:
		return nil, -1
	}
}

func ReadTDFStringValue(data []byte, offset int) (string, int) {
	length, next := ReadTDFInteger(data, offset)

	if next < 0 || length <= 0 {
		return "", -1
	}

	if int64(len(data)-next) < length {
		return "", -1
	}

	end := next + int(length)

	valueEnd := end

	if data[end-1] == 0 {
		valueEnd--
	}

	return string(data[next:valueEnd]), end
}

func skipTDFStruct(data []byte, offset int) int {
	for offset < len(data) {
		if data[offset] == 0x00 {
			return offset + 1
		}

		_, next := ReadTag(data, offset)

		if next < 0 || next >= len(data) {
			return -1
		}

		offset = next

		if offset >= len(data) {
			return -1
		}

		wireType := data[offset]
		offset++

		offset = skipTDFValue(data, offset, wireType)

		if offset < 0 || offset > len(data) {
			return -1
		}
	}

	return -1
}

func skipTDFValue(data []byte, offset int, wireType byte) int {
	switch wireType {
	case wireInt:
		_, next := ReadTDFInteger(data, offset)
		return next

	case wireString:
		length, next := ReadTDFInteger(data, offset)

		if next < 0 || length < 0 || int64(len(data)-next) < length {
			return -1
		}

		return next + int(length)

	case wireBlob:
		length, next := ReadTDFInteger(data, offset)

		if next < 0 || length < 0 || int64(len(data)-next) < length {
			return -1
		}

		return next + int(length)

	case wireStruct:
		return skipTDFStruct(data, offset)

	case wireList:
		if offset >= len(data) {
			return -1
		}

		elementType := data[offset]
		offset++

		count, next := ReadTDFInteger(data, offset)

		if next < 0 || count < 0 {
			return -1
		}

		offset = next

		for i := int64(0); i < count; i++ {
			offset = skipTDFValue(data, offset, elementType)

			if offset < 0 {
				return -1
			}
		}

		return offset

	case wireMap:
		if offset+2 > len(data) {
			return -1
		}

		keyType := data[offset]
		valueType := data[offset+1]
		offset += 2

		count, next := ReadTDFInteger(data, offset)

		if next < 0 || count < 0 {
			return -1
		}

		offset = next

		for i := int64(0); i < count; i++ {
			offset = skipTDFValue(data, offset, keyType)

			if offset < 0 {
				return -1
			}

			offset = skipTDFValue(data, offset, valueType)

			if offset < 0 {
				return -1
			}
		}

		return offset

	case wireUnion:
		if offset >= len(data) {
			return -1
		}

		activeMember := data[offset]
		offset++

		if activeMember == 0x7F {
			return offset
		}

		unionTag, next := ReadTag(data, offset)

		if next < 0 || unionTag != "VALU" {
			return -1
		}

		offset = next
		return skipTDFStruct(data, offset)

	default:
		return -1
	}
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

func DebugTDF(label string, data []byte) {
	fields := ReadTDF(data)

	logger.Debug("TDF %s: %d fields", label, len(fields))

	for _, field := range fields {
		logger.Debug("TAG=%q TYPE=0x%02X (%s)", field.Tag, field.Type, TDFTypeName(field.Type),)

		debugTDFValue("    ", field.Type, field.Value)
	}
}

func debugTDFValue(indent string, t byte, value interface{}) {
	switch v := value.(type) {
	case string:
		logger.Debug("%sVALUE=%q", indent, v)

	case bool:
		logger.Debug("%sVALUE=%v", indent, v)

	case int64:
		logger.Debug("%sVALUE=%d (0x%X)", indent, v, uint64(v))

	case []byte:
		logger.Debug("%sRAW=% X", indent, v)

		if t == wireStruct {
			DebugTDFNested(indent+"    ", v)
		}

	case [][]byte:
		logger.Debug("%sCOUNT=%d", indent, len(v))

		for i, element := range v {
			logger.Debug("%s[%d] RAW=% X", indent, i, element)

			if len(element) > 0 {
				DebugTDFNested(indent+"    ", element)
			}
		}

	case TDFUnion:
		logger.Debug("%sACTIVE_MEMBER=0x%02X", indent, v.ActiveMember,)
		logger.Debug("%sDATA=% X", indent, v.Data)

		if len(v.Data) > 0 {
			DebugTDFNested(indent+"    ", v.Data)
		}

	default:
		logger.Debug("%sVALUE=%v", indent, value)
	}
}

func DebugTDFNested(indent string, data []byte) {
	fields := ReadTDF(data)

	for i, field := range fields {
		logger.Debug("%s[%02d] TAG=%-4q TYPE=0x%02X (%s)", indent, i, field.Tag, field.Type, TDFTypeName(field.Type),)

		switch v := field.Value.(type) {
		case string:
			logger.Debug("%s    VALUE=%q", indent, v)

		case int64:
			logger.Debug("%s    VALUE=%d (0x%X)", indent, v, uint64(v),)

		case []byte:
			logger.Debug("%s    RAW=% X", indent, v)

			if field.Type == wireStruct {
				DebugTDFNested(indent+"    ", v)
			}

		case [][]byte:
			logger.Debug("%s    COUNT=%d", indent, len(v))

			for j, element := range v {
				logger.Debug("%s    [%d] RAW=% X", indent, j, element,)
			}

		case TDFUnion:
			logger.Debug("%s    ACTIVE_MEMBER=0x%02X", indent, v.ActiveMember,)
			logger.Debug("%s    DATA=% X", indent, v.Data,)

			if len(v.Data) > 0 {
				DebugTDFNested(indent+"        ", v.Data)
			}

		default:
			logger.Debug("%s    VALUE=%v", indent, field.Value,)
		}
	}
}

func TDFTypeName(t byte) string {
	switch t {
	case wireInt:
		return "INT/BOOL"
	case wireString:
		return "STRING"
	case wireBlob:
		return "BLOB"
	case wireStruct:
		return "STRUCT"
	case wireList:
		return "LIST"
	case wireMap:
		return "MAP"
	case wireUnion:
		return "UNION"
	case wireUnknown:
		return "VARIABLE"
	case wireVector:
		return "VECTOR"
	case wireVector3:
		return "VECTOR3"
	default:
		return "UNKNOWN"
	}
}

func fmtTDFBytes(data []byte) string {
	return fmt.Sprintf("% X", data)
}
