package blaze

import (
	"encoding/binary"
	"fmt"
)

const (
	PacketTypeRequest  = 0x00
	PacketTypeResponse = 0x01
)

type Packet struct {
	Component uint16
	Command   uint16
	Type      uint8
	MessageId uint32
	Payload   []byte
}

// Blaze packet format:
//
// Component  (2 bytes)
// Command    (2 bytes)
// Type       (1 byte)
// Message ID (4 bytes)
// Payload
//

func Parse(data []byte) Packet {
	p := Packet{}
	if len(data) < 9 {
		return p
	}

	p.Component = binary.BigEndian.Uint16(data[0:2])
	p.Command = binary.BigEndian.Uint16(data[2:4])
	p.Type = data[4]
	p.MessageId = binary.BigEndian.Uint32(data[5:9])
	p.Payload = data[9:]
	return p
}

func (p Packet) Dump() {
	fmt.Printf("[BLAZE] Component=%d Command=%d Type=%02X Msg=%d Payload=%d bytes\n", p.Component, p.Command, p.Type, p.MessageId, len(p.Payload),)
	fmt.Printf("% X\n", p.Payload,)
}