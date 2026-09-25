package blaze

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
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

	result := uint32(uint8(tag[0])-32) << 26

	if len(tag) > 1 {
		result |= uint32((uint8(tag[1])-32)&0x3F) << 20
	}
	if len(tag) > 2 {
		result |= uint32((uint8(tag[2])-32)&0x3F) << 14
	}
	if len(tag) > 3 {
		result |= uint32((uint8(tag[3])-32)&0x3F) << 8
	}

	var data [4]byte
	binary.BigEndian.PutUint32(data[:], result)
	buf.Write(data[:3])
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

	var cur byte

	if value >= 0 {
		cur = byte(value&0x3F) | 0x80
	} else {
		value = -value
		cur = byte(value&0x3F) | 0xC0
	}

	for i := value >> 6; i > 0; i >>= 7 {
		buf.WriteByte(cur)
		cur = byte(i&0x7F) | 0x80
	}

	buf.WriteByte(cur & 0x7F)
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

	if value == 0 {
		buf.WriteByte(0)
		return
	}

	cur := byte(value&0x3F) | 0x80

	for i := value >> 6; i > 0; i >>= 7 {
		buf.WriteByte(cur)
		cur = byte(i&0x7F) | 0x80
	}

	buf.WriteByte(cur & 0x7F)
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
		buf.Write(element)
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
	}
}

func WriteMap(buf *bytes.Buffer, tag string, keyType byte, valueType byte, values [][]byte) {
	WriteTag(buf, tag)
	buf.WriteByte(wireMap)
	buf.WriteByte(tdfWireType(keyType))
	buf.WriteByte(tdfWireType(valueType))

	count := len(values) / 2
	WriteTDFInteger(buf, int64(count))

	for i := 0; i+1 < len(values); i += 2 {
		key := values[i]
		value := values[i+1]

		WriteTDFInteger(buf, int64(len(key)+1))
		buf.Write(key)
		buf.WriteByte(0)

		WriteTDFInteger(buf, int64(len(value)+1))
		buf.Write(value)
		buf.WriteByte(0)
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

	if secure {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}

	WriteTag(&buf, "XDNS")
	buf.WriteByte(wireInt)
	WriteTDFInteger(&buf, 0)

	return buf.Bytes()
}

func ReadTDF(data []byte) []TDF {
	var result []TDF
	offset := 0

	for offset+3 <= len(data) {
		tag, next := ReadTag(data, offset)
		if next < 0 {
			return result
		}
		offset = next

		if offset >= len(data) {
			return result
		}

		t := data[offset]
		offset++

		switch t {
		case wireInt:
			value, next := ReadTDFInteger(data, offset)
			if next < 0 {
				return result
			}
			offset = next
			result = append(result, TDF{
				Tag:   tag,
				Type:  wireInt,
				Value: value,
			})

		case wireString:
			value, next := ReadTDFStringValue(data, offset)
			if next < 0 {
				return result
			}
			offset = next
			result = append(result, TDF{
				Tag:   tag,
				Type:  wireString,
				Value: value,
			})

		case wireBlob:
			length, next := ReadTDFInteger(data, offset)
			if next < 0 || length < 0 || int64(len(data)-next) < length {
				return result
			}

			offset = next
			value := make([]byte, int(length))
			copy(value, data[offset:offset+int(length)])
			offset += int(length)

			result = append(result, TDF{
				Tag:   tag,
				Type:  wireBlob,
				Value: value,
			})

		case wireStruct:
			start := offset
			offset = skipTDFStruct(data, offset)
			if offset > len(data) {
				return result
			}

			value := make([]byte, offset-start)
			copy(value, data[start:offset])

			result = append(result, TDF{
				Tag:   tag,
				Type:  wireStruct,
				Value: value,
			})

		case wireList:
			if offset >= len(data) {
				return result
			}

			elementType := data[offset]
			offset++

			count, next := ReadTDFInteger(data, offset)
			if next < 0 || count < 0 {
				return result
			}
			offset = next

			elements := make([][]byte, 0, int(count))

			for i := int64(0); i < count; i++ {
				start := offset
				offset = skipTDFValue(data, offset, elementType)

				if offset > len(data) {
					return result
				}

				element := make([]byte, offset-start)
				copy(element, data[start:offset])
				elements = append(elements, element)
			}

			result = append(result, TDF{
				Tag:   tag,
				Type:  wireList,
				Value: elements,
			})

		case wireMap:
			if offset+2 > len(data) {
				return result
			}

			keyType := data[offset]
			valueType := data[offset+1]
			_ = keyType
			_ = valueType
			offset += 2

			count, next := ReadTDFInteger(data, offset)
			if next < 0 || count < 0 {
				return result
			}
			offset = next

			entries := make([][]byte, 0, int(count)*2)

			for i := int64(0); i < count; i++ {
				keyLen, next := ReadTDFInteger(data, offset)
				if next < 0 || keyLen < 0 || int64(len(data)-next) < keyLen {
					return result
				}
				offset = next

				key := make([]byte, int(keyLen))
				copy(key, data[offset:offset+int(keyLen)])
				offset += int(keyLen)
				entries = append(entries, key)

				valueLen, next := ReadTDFInteger(data, offset)
				if next < 0 || valueLen < 0 || int64(len(data)-next) < valueLen {
					return result
				}
				offset = next

				value := make([]byte, int(valueLen))
				copy(value, data[offset:offset+int(valueLen)])
				offset += int(valueLen)
				entries = append(entries, value)
			}

			result = append(result, TDF{
				Tag:   tag,
				Type:  wireMap,
				Value: entries,
			})

		case wireUnion:
			if offset >= len(data) {
				return result
			}

			activeMember := data[offset]
			offset++

			if activeMember != 0x7F {
				unionTag, next := ReadTag(data, offset)
				if next < 0 {
					return result
				}
				offset = next

				if unionTag != "VALU" {
					return result
				}

				start := offset
				offset = skipTDFStruct(data, offset)
				if offset > len(data) {
					return result
				}

				value := make([]byte, offset-start)
				copy(value, data[start:offset])

				result = append(result, TDF{
					Tag:  tag,
					Type: wireUnion,
					Value: struct {
						ActiveMember byte
						Data         []byte
					}{
						ActiveMember: activeMember,
						Data:         value,
					},
				})
			} else {
				result = append(result, TDF{
					Tag:   tag,
					Type:  wireUnion,
					Value: activeMember,
				})
			}

		case wireUnknown, wireVector, wireVector3:
			fmt.Printf("Unsupported wire TDF type %02X\n", t)
			return result

		default:
			fmt.Printf("Unknown TDF wire type %02X\n", t)
			return result
		}
	}

	return result
}

func ReadTag(data []byte, offset int) (string, int) {
	if offset+3 > len(data) {
		return "", -1
	}

	var raw [4]byte
	copy(raw[:3], data[offset:offset+3])

	value := binary.BigEndian.Uint32(raw[:])

	var tag [4]byte
	length := 4

	v := value & 0x3F00
	if v != 0 {
		tag[3] = byte((value>>8)&0x3F) + 32
	} else {
		length = 3
	}

	v = (value >> 14) & 0x3F
	if v != 0 {
		tag[2] = byte(v) + 32
	} else {
		length = 2
	}

	v = (value >> 20) & 0x3F
	if v != 0 {
		tag[1] = byte(v) + 32
	} else {
		length = 1
	}

	v = value >> 26
	if v != 0 {
		tag[0] = byte(v) + 32
	} else {
		length = 0
	}

	return string(tag[:length]), offset + 3
}

func ReadTDFInteger(data []byte, offset int) (int64, int) {
	if offset >= len(data) {
		return 0, -1
	}

	first := data[offset]
	offset++

	negative := (first & 0x40) != 0
	readNext := (first & 0x80) != 0

	var value uint64 = uint64(first & 0x3F)
	shift := uint(6)

	for readNext {
		if offset >= len(data) {
			return 0, -1
		}

		b := data[offset]
		offset++

		value |= uint64(b&0x7F) << shift
		shift += 7
		readNext = b&0x80 != 0
	}

	if negative {
		if value == 0 {
			return -9223372036854775808, offset
		}
		return -int64(value), offset
	}

	return int64(value), offset
}

func ReadTDFStringValue(data []byte, offset int) (string, int) {
	length, next := ReadTDFInteger(data, offset)
	if next < 0 || length <= 0 || int64(len(data)-next) < length {
		return "", -1
	}

	offset = next
	valueLength := int(length)

	if data[offset+valueLength-1] == 0 {
		valueLength--
	}

	value := string(data[offset : offset+valueLength])
	return value, offset + int(length)
}

func skipTDFStruct(data []byte, offset int) int {
	for offset+3 <= len(data) {
		if data[offset] == 0x00 {
			return offset + 1
		}

		_, next := ReadTag(data, offset)
		if next < 0 || next >= len(data) {
			return len(data) + 1
		}

		offset = next

		if offset >= len(data) {
			return len(data) + 1
		}

		t := data[offset]
		offset++

		offset = skipTDFValue(data, offset, t)

		if offset > len(data) {
			return len(data) + 1
		}
	}

	return len(data) + 1
}

func skipTDFValue(data []byte, offset int, t byte) int {
	switch t {
	case wireInt:
		_, next := ReadTDFInteger(data, offset)
		return next

	case wireString:
		length, next := ReadTDFInteger(data, offset)
		if next < 0 || length < 0 || int64(len(data)-next) < length {
			return len(data) + 1
		}
		return next + int(length)

	case wireBlob:
		length, next := ReadTDFInteger(data, offset)
		if next < 0 || length < 0 || int64(len(data)-next) < length {
			return len(data) + 1
		}
		return next + int(length)

	case wireStruct:
		return skipTDFStruct(data, offset)

	case wireList:
		if offset >= len(data) {
			return len(data) + 1
		}

		elementType := data[offset]
		offset++

		count, next := ReadTDFInteger(data, offset)
		if next < 0 || count < 0 {
			return len(data) + 1
		}
		offset = next

		for i := int64(0); i < count; i++ {
			offset = skipTDFValue(data, offset, elementType)
			if offset > len(data) {
				return len(data) + 1
			}
		}

		return offset

	case wireMap:
		if offset+2 > len(data) {
			return len(data) + 1
		}

		keyType := data[offset]
		valueType := data[offset+1]
		offset += 2

		count, next := ReadTDFInteger(data, offset)
		if next < 0 || count < 0 {
			return len(data) + 1
		}
		offset = next

		for i := int64(0); i < count; i++ {
			keyLen, next := ReadTDFInteger(data, offset)
			if next < 0 || keyLen < 0 || int64(len(data)-next) < keyLen {
				return len(data) + 1
			}
			offset = next + int(keyLen)

			valueLen, next := ReadTDFInteger(data, offset)
			if next < 0 || valueLen < 0 || int64(len(data)-next) < valueLen {
				return len(data) + 1
			}
			offset = next + int(valueLen)

			_ = keyType
			_ = valueType
		}

		return offset

	case wireUnion:
		if offset >= len(data) {
			return len(data) + 1
		}

		activeMember := data[offset]
		offset++

		if activeMember == 0x7F {
			return offset
		}

		_, next := ReadTag(data, offset)
		if next < 0 {
			return len(data) + 1
		}

		offset = next
		return skipTDFStruct(data, offset)

	default:
		return len(data) + 1
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
	fmt.Printf("Raw (%d bytes): % X\n", len(data), data)

	fields := ReadTDF(data)

	for i, field := range fields {
		fmt.Printf("[%02d] TAG=%-4q TYPE=0x%02X (%s)\n", i, field.Tag, field.Type, TDFTypeName(field.Type),)
		debugTDFValue("    ", field.Type, field.Value)
	}
}

func debugTDFValue(indent string, t byte, value interface{}) {
	switch v := value.(type) {
	case string:
		fmt.Printf("%sVALUE=%q\n", indent, v)

	case bool:
		fmt.Printf("%sVALUE=%v\n", indent, v)

	case int64:
		fmt.Printf("%sVALUE=%d (0x%X)\n", indent, v, uint64(v))

	case []byte:
		fmt.Printf("%sRAW=% X\n", indent, v)
		if t == wireStruct || t == wireUnion {
			DebugTDFNested(indent+"    ", v)
		}

	case [][]byte:
		fmt.Printf("%sCOUNT=%d\n", indent, len(v))
		for i, element := range v {
			fmt.Printf("%s[%d] RAW=% X\n", indent, i, element)
			if len(element) > 0 {
				DebugTDFNested(indent+"    ", element)
			}
		}

	case struct {
		ActiveMember byte
		Data         []byte
	}:
		fmt.Printf("%sACTIVE_MEMBER=0x%02X\n", indent, v.ActiveMember)
		fmt.Printf("%sDATA=% X\n", indent, v.Data)
		fmt.Printf("%sDECODED UNION DATA:\n", indent)
		DebugTDFNested(indent+"    ", v.Data)

	default:
		fmt.Printf("%sVALUE=%v\n", indent, value)
	}
}

func DebugTDFNested(indent string, data []byte) {
	fields := ReadTDF(data)

	for i, field := range fields {
		fmt.Printf("%s[%02d] TAG=%-4q TYPE=0x%02X (%s)\n", indent, i, field.Tag, field.Type, TDFTypeName(field.Type),)

		switch v := field.Value.(type) {
		case string:
			fmt.Printf("%sVALUE=%q\n", indent, v)
		case bool:
			fmt.Printf("%sVALUE=%v\n", indent, v)
		case int64:
			fmt.Printf("%sVALUE=%d (0x%X)\n", indent, v, uint64(v))

		case []byte:
			fmt.Printf("%sRAW=% X\n", indent, v)
			if field.Type == wireStruct {
				DebugTDFNested(indent+"", v)
			}

		case [][]byte:
			fmt.Printf("%sCOUNT=%d\n", indent, len(v))
			for j, element := range v {
				fmt.Printf("%s[%d] RAW=% X\n", indent, j, element)
				if len(element) > 0 {
					DebugTDFNested(indent+"", element)
				}
			}

		case struct {
			ActiveMember byte
			Data         []byte
		}:
			fmt.Printf("%sACTIVE_MEMBER=0x%02X\n", indent, v.ActiveMember)
			fmt.Printf("%sDATA=% X\n", indent, v.Data)
			DebugTDFNested(indent+"", v.Data)

		default:
			fmt.Printf("%sVALUE=%v\n", indent, field.Value)
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
