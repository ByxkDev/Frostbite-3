package blaze

import (
	"encoding/binary"
	"fmt"
)

const (
	PacketTypeRequest  uint16 = 0x0000
	PacketTypeResponse uint16 = 0x1000
	PacketTypeNotify   uint16 = 0x2000
)

const blazeHeaderSize = 12

type Packet struct {
	Size      uint16
	Component uint16
	Command   uint16
	Error     uint16
	Type      uint16
	MessageId uint32
	Payload   []byte
}

func Parse(data []byte) Packet {
	p := Packet{}

	if len(data) < blazeHeaderSize {
		return p
	}

	p.Size = binary.BigEndian.Uint16(data[0:2])
	p.Component = binary.BigEndian.Uint16(data[2:4])
	p.Command = binary.BigEndian.Uint16(data[4:6])
	p.Error = binary.BigEndian.Uint16(data[6:8])
	p.Type = binary.BigEndian.Uint16(data[8:10])
	p.MessageId = uint32(binary.BigEndian.Uint16(data[10:12]))

	payloadLen := int(p.Size)
	if payloadLen > len(data)-blazeHeaderSize {
		payloadLen = len(data) - blazeHeaderSize
	}

	if payloadLen < 0 {
		payloadLen = 0
	}

	p.Payload = data[blazeHeaderSize : blazeHeaderSize+payloadLen]
	return p
}

func (p Packet) Dump() {
	fmt.Printf("[BLAZE] Size=%d Component=%d Command=%d Error=%d Type=0x%04X MessageId=%d Payload=%d bytes\n", p.Size, p.Component, p.Command, p.Error, p.Type, p.MessageId, len(p.Payload),)
	fmt.Printf("[BLAZE] Payload: % X\n", p.Payload)
}

func EncodePacket(component uint16, command uint16, packetType uint16, messageID uint32, payload []byte) []byte {
	if len(payload) > 0xFFFF {
		panic("Blaze payload exceeds uint16 size")
	}

	if messageID > 0xFFFF {
		panic("Blaze message ID exceeds uint16")
	}

	packetSize := uint16(len(payload))
	data := make([]byte, blazeHeaderSize+len(payload))

	binary.BigEndian.PutUint16(data[0:2], packetSize)
	binary.BigEndian.PutUint16(data[2:4], component)
	binary.BigEndian.PutUint16(data[4:6], command)
	binary.BigEndian.PutUint16(data[6:8], 0)
	binary.BigEndian.PutUint16(data[8:10], packetType)
	binary.BigEndian.PutUint16(data[10:12], uint16(messageID))

	copy(data[blazeHeaderSize:], payload)
	return data
}
