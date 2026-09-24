package blaze

import (
	"bytes"
	"encoding/binary"
)

const (
	RedirectorComponent uint16 = 1
	GetServerInstance   uint16 = 1
)

const (
	GameServerIP   = "151.xxx.xxx.xx"
	GameServerPort uint16 = 33152
)

func BuildGetServerInstanceResponse(messageID uint32) []byte {
	payload := bytes.NewBuffer(nil)
	// ADDR = ServerAddress
	address := bytes.NewBuffer(nil)
	// IpAddress
	WriteTDF(address, "HOST", GameServerIP)
	WriteUInt32(address, "IP", GameServerIPUInt())
	WriteUInt16(address, "PORT", GameServerPort)
	WriteStruct(payload, "ADDR", address.Bytes())
	// AREM = AddressRemapEntry[]
	WriteList(payload, "AREM", TDF_STRUCT, nil)
	// DNST = default DNS address
	WriteUInt32(payload, "DNST", 0)
	// MESS = messages[]
	messages := [][]byte{
		encodeStringElement("@ByxkDev"),
		encodeStringElement("Welcome To Syntax!"),
	}
	WriteList(payload, "MESS", TDF_STRING, messages)
	// NREM = NameRemapEntry[]
	WriteList(payload, "NREM", TDF_STRUCT, nil)
	// SECU = secure
	WriteBool(payload, "SECU", false)

	return EncodePacket(RedirectorComponent, GetServerInstance, 0, messageID, payload.Bytes(),)
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
