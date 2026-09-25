package redirector

import (
	"bytes"
	"encoding/binary"

	"bf4/blaze"
)

const (
	RedirectorComponent uint16 = 1
	GetServerInstance   uint16 = 1
)

const (
	BlazeServerIP   = "151.xxx.xxx.xx"
	BlazeServerPort uint16 = 33152
)

func BuildGetServerInstanceResponse(messageID uint32, clientType string) []byte {
	payload := bytes.NewBuffer(nil)

	address := bytes.NewBuffer(nil)

	blaze.WriteTDF(address, "HOST", BlazeServerIP)
	blaze.WriteUInt32(address, "IP", GameServerIPUInt())
	blaze.WriteUInt16(address, "PORT", BlazeServerPort)
	blaze.WriteStruct(payload, "ADDR", address.Bytes())

	blaze.WriteList(payload, "AREM", blaze.TDF_STRUCT, nil)
	blaze.WriteUInt32(payload, "DNST", 0)

	messages := [][]byte{
		encodeStringElement("Hello World"),
		encodeStringElement("Welcome To Battlefield 4!"),
	}

	blaze.WriteList(payload, "MESS", blaze.TDF_STRING, messages)
	blaze.WriteList(payload, "NREM", blaze.TDF_STRUCT, nil)
	blaze.WriteBool(payload, "SECU", false)

	return blaze.EncodePacket(RedirectorComponent, GetServerInstance, 0, messageID, payload.Bytes(),)
}

func encodeStringElement(value string) []byte {
	buf := bytes.NewBuffer(nil)

	binary.Write(buf, binary.BigEndian, uint32(len(value)+1))
	buf.WriteString(value)
	buf.WriteByte(0)

	return buf.Bytes()
}

func IPToUInt(ip [4]byte) uint32 {
	return binary.BigEndian.Uint32(ip[:])
}

func GameServerIPUInt() uint32 {
	return IPToUInt([4]byte{151, 244, 72, 66})
}
