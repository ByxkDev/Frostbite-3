package blaze

import (
	"bytes"
	"encoding/binary"
)

func EncodePacket(component uint16, command uint16, packetType uint8, messageId uint32, payload []byte,) []byte {
	buf := bytes.NewBuffer(nil)
	binary.Write(buf, binary.BigEndian, component,)
	binary.Write(buf, binary.BigEndian, command,)
	buf.WriteByte(packetType)
	binary.Write(buf, binary.BigEndian, messageId,)
	buf.Write(payload)
	return buf.Bytes()
}