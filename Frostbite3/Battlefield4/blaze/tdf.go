package blaze

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	TDF_INT8   byte = 0x00
	TDF_INT16  byte = 0x01
	TDF_INT32  byte = 0x02
	TDF_INT64  byte = 0x03
	TDF_UINT8  byte = 0x04
	TDF_UINT16 byte = 0x05
	TDF_UINT32 byte = 0x06
	TDF_UINT64 byte = 0x07
	TDF_STRING byte = 0x08
	TDF_BLOB   byte = 0x09
	TDF_STRUCT byte = 0x0A
	TDF_LIST   byte = 0x0B
	TDF_MAP    byte = 0x0C
	TDF_UNION  byte = 0x0D
	TDF_VARIABLE byte = 0x0E
	TDF_GENERIC_TYPE byte = 0x0F
	TDF_GENERIC_TYPE2 byte = 0x10
	TDF_FLOAT  byte = 0x11
	TDF_TIME   byte = 0x12
	TDF_BOOL   byte = 0x13
)

type TDF struct {
	Tag   string
	Type  byte
	Value interface{}
}

func WriteTag(buf *bytes.Buffer, tag string) {
	for i := 0; i < 4; i++ {
		if i < len(tag) {
			buf.WriteByte(tag[i])
		} else {
			buf.WriteByte(0)
		}
	}
}

func WriteTDF(buf *bytes.Buffer, tag string, value string) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_STRING)
	binary.Write(buf, binary.BigEndian, uint32(len(value)+1))
	buf.WriteString(value)
	buf.WriteByte(0)
}

func WriteInt8(buf *bytes.Buffer, tag string, value int8) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_INT8)
	buf.WriteByte(byte(value))
}

func WriteInt16(buf *bytes.Buffer, tag string, value int16) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_INT16)
	binary.Write(buf, binary.BigEndian, value)
}

func WriteInt32(buf *bytes.Buffer, tag string, value int32) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_INT32)
	binary.Write(buf, binary.BigEndian, value)
}

func WriteInt64(buf *bytes.Buffer, tag string, value int64) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_INT64)
	binary.Write(buf, binary.BigEndian, value)
}

func WriteUInt8(buf *bytes.Buffer, tag string, value uint8) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_UINT8)
	buf.WriteByte(value)
}

func WriteUInt16(buf *bytes.Buffer, tag string, value uint16) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_UINT16)
	binary.Write(buf, binary.BigEndian, value)
}

func WriteUInt32(buf *bytes.Buffer, tag string, value uint32) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_UINT32)
	binary.Write(buf, binary.BigEndian, value)
}

func WriteUInt64(buf *bytes.Buffer, tag string, value uint64) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_UINT64)
	binary.Write(buf, binary.BigEndian, value)
}

func WriteBool(buf *bytes.Buffer, tag string, value bool) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_BOOL)

	if value {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
}

func WriteBlob(buf *bytes.Buffer, tag string, value []byte) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_BLOB)
	binary.Write(buf, binary.BigEndian, uint32(len(value)))
	buf.Write(value)
}

func WriteStruct(buf *bytes.Buffer, tag string, data []byte) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_STRUCT)
	buf.Write(data)
}

func WriteList(buf *bytes.Buffer, tag string, elementType byte, elements [][]byte) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_LIST)
	buf.WriteByte(elementType)
	binary.Write(buf, binary.BigEndian, uint32(len(elements)))

	for _, element := range elements {
		buf.Write(element)
	}
}

func WriteMap(buf *bytes.Buffer, tag string, keyType byte, valueType byte, entries [][]byte) {
	WriteTag(buf, tag)
	buf.WriteByte(TDF_MAP)
	buf.WriteByte(keyType)
	buf.WriteByte(valueType)
	binary.Write(buf, binary.BigEndian, uint32(len(entries)))

	for _, entry := range entries {
		buf.Write(entry)
	}
}

func ReadTDF(data []byte) []TDF {
	var result []TDF
	offset := 0

	for offset+5 <= len(data) {
		tag := string(data[offset : offset+4])
		offset += 4

		t := data[offset]
		offset++

		switch t {
		case TDF_INT8:
			if offset+1 > len(data) {
				return result
			}

			value := int8(data[offset])
			offset++

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_INT16:
			if offset+2 > len(data) {
				return result
			}

			value := int16(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_INT32:
			if offset+4 > len(data) {
				return result
			}

			value := int32(binary.BigEndian.Uint32(data[offset : offset+4]))
			offset += 4

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_INT64:
			if offset+8 > len(data) {
				return result
			}

			value := int64(binary.BigEndian.Uint64(data[offset : offset+8]))
			offset += 8

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_UINT8:
			if offset+1 > len(data) {
				return result
			}

			value := data[offset]
			offset++

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_UINT16:
			if offset+2 > len(data) {
				return result
			}

			value := binary.BigEndian.Uint16(data[offset : offset+2])
			offset += 2

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_UINT32:
			if offset+4 > len(data) {
				return result
			}

			value := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_UINT64:
			if offset+8 > len(data) {
				return result
			}

			value := binary.BigEndian.Uint64(data[offset : offset+8])
			offset += 8

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_STRING:
			if offset+4 > len(data) {
				return result
			}

			length := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4

			if length == 0 || offset+int(length) > len(data) {
				return result
			}

			valueLength := int(length)

			if data[offset+valueLength-1] == 0 {
				valueLength--
			}

			value := string(data[offset : offset+valueLength])
			offset += int(length)

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_BLOB:
			if offset+4 > len(data) {
				return result
			}

			length := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4

			if offset+int(length) > len(data) {
				return result
			}

			value := make([]byte, length)
			copy(value, data[offset:offset+int(length)])
			offset += int(length)

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_BOOL:
			if offset+1 > len(data) {
				return result
			}

			value := data[offset] != 0
			offset++

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_STRUCT:
			start := offset
			offset = skipTDFStruct(data, offset)

			if offset > len(data) {
				return result
			}

			value := make([]byte, offset-start)
			copy(value, data[start:offset])

			result = append(result, TDF{
				Tag:   tag,
				Type:  t,
				Value: value,
			})

		case TDF_LIST:
			if offset+5 > len(data) {
				return result
			}

			elementType := data[offset]
			offset++

			count := binary.BigEndian.Uint32(data[offset : offset+4])
			offset += 4

			elements := make([][]byte, 0, count)

			for i := uint32(0); i < count; i++ {
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
				Type:  t,
				Value: elements,
			})

		default:
			fmt.Printf("Unknown TDF type %02X\n", t)
			return result
		}
	}

	return result
}

func skipTDFStruct(data []byte, offset int) int {
	for offset+5 <= len(data) {
		offset += 4

		t := data[offset]
		offset++

		if t == TDF_STRUCT {
			offset = skipTDFStruct(data, offset)
			continue
		}

		offset = skipTDFValue(data, offset, t)

		if offset > len(data) {
			return len(data) + 1
		}
	}

	return offset
}

func skipTDFValue(data []byte, offset int, t byte) int {
	switch t {
	case TDF_INT8, TDF_UINT8, TDF_BOOL:
		return offset + 1

	case TDF_INT16, TDF_UINT16:
		return offset + 2

	case TDF_INT32, TDF_UINT32:
		return offset + 4

	case TDF_INT64, TDF_UINT64:
		return offset + 8

	case TDF_STRING, TDF_BLOB:
		if offset+4 > len(data) {
			return len(data) + 1
		}

		length := binary.BigEndian.Uint32(data[offset : offset+4])
		return offset + 4 + int(length)

	case TDF_STRUCT:
		return skipTDFStruct(data, offset)

	default:
		return len(data) + 1
	}
}