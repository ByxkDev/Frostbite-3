package redirector

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"bf4/blaze"
)

const (
	RedirectorComponent uint16 = 5
	GetServerInstance   uint16 = 1
)

const (
	BlazeServerIP   = "151.xxx.xxx.xx"
	BlazeServerPort = uint32(33152)
)

const (
	tdfInt    byte = 0x00
	tdfString byte = 0x01
	tdfStruct byte = 0x03
	tdfUnion  byte = 0x06
)

func BuildGetServerInstanceResponse(messageID uint32, clientType string) []byte {
	payload := bytes.NewBuffer(nil)
	// ADDR union
	writeTag(payload, "ADDR")
	payload.WriteByte(tdfUnion)
	payload.WriteByte(0x00)
	// VALU struct
	writeTag(payload, "VALU")
	payload.WriteByte(tdfStruct)
	// HOST string
	writeString(payload, "HOST", BlazeServerIP)
	// IP int
	writeInt(payload, "IP  ", blaze.IPToUInt(GameServerIP))
	// PORT int
	writeInt(payload, "PORT", BlazeServerPort)
	// End ADDR union
	payload.WriteByte(0x00)
	// SECU int
	writeInt(payload, "SECU", 0)
	// XDNS int
	writeInt(payload, "XDNS", 0)

	fmt.Printf("[REDIRECTOR] Client Type : %q\n", clientType)
	fmt.Printf("[REDIRECTOR] Message ID  : %d\n", messageID)
	fmt.Printf("[REDIRECTOR] Server IP   : %s\n", BlazeServerIP)
	fmt.Printf("[REDIRECTOR] Server Port : %d\n", BlazeServerPort)
	fmt.Printf("[REDIRECTOR] Secure      : false\n")
	fmt.Printf("[REDIRECTOR] Payload RAW : % X\n", payload.Bytes())

	blaze.DebugTDF("GetServerInstanceResponse", payload.Bytes())

	packet := blaze.EncodePacket(RedirectorComponent, GetServerInstance, blaze.PacketTypeResponse, messageID, payload.Bytes(),)

	fmt.Printf("[REDIRECTOR] Final Packet (%d bytes): % X\n", len(packet), packet)
	return packet
}

func writeTag(buf *bytes.Buffer, tag string) {
	if len(tag) != 4 {
		panic("Blaze TDF tags must be exactly 4 characters")
	}

	var value uint32

	for i := 0; i < 4; i++ {
		value = (value << 6) | uint32(byte(tag[i])-0x20)
	}

	buf.Write([]byte{
		byte(value >> 16),
		byte(value >> 8),
		byte(value),
	})
}

func writeString(buf *bytes.Buffer, tag string, value string) {
	writeTag(buf, tag)
	buf.WriteByte(tdfString)
	writeTDFInteger(buf, int64(len(value)+1))
	buf.WriteString(value)
	buf.WriteByte(0x00)
}

func writeInt(buf *bytes.Buffer, tag string, value uint32) {
	writeTag(buf, tag)
	buf.WriteByte(tdfInt)
	writeTDFInteger(buf, int64(value))
}

func writeTDFInteger(buf *bytes.Buffer, value int64) {
	if value < 0 {
		panic("negative Blaze integer is not supported")
	}

	if value < 0x32 {
		buf.WriteByte(byte(value))
		return
	}

	b := byte((value & 0x3F) | 0x80)
	buf.WriteByte(b)

	value >>= 6

	for value > 0x80 {
		buf.WriteByte(byte((value & 0x7F) | 0x80))
		value >>= 7
	}

	buf.WriteByte(byte(value))
}

func encodePacketSize(payload []byte) uint16 {
	if len(payload) > 0xFFFF {
		panic("Blaze payload too large")
	}

	var size uint16
	binary.BigEndian.PutUint16([]byte{
		byte(size >> 8),
		byte(size),
	}, uint16(len(payload)))

	return uint16(len(payload))
}
