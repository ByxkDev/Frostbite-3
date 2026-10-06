package blaze

import (
	"encoding/binary"

	"bf4/logger"
)

const (
	PacketTypeRequest  uint16 = 0x0000
	PacketTypeResponse uint16 = 0x1000
	PacketTypeNotify   uint16 = 0x2000

	blazeHeaderSize = 12
)

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
	var p Packet

	if len(data) < blazeHeaderSize {
		logger.Warn("BLAZE packet too small: %d bytes (need at least %d)", len(data), blazeHeaderSize)
		logger.Hex(logger.LevelDebug, "BLAZE SHORT PACKET", data)
		return p
	}

	p.Size = binary.BigEndian.Uint16(data[0:2])
	p.Component = binary.BigEndian.Uint16(data[2:4])
	p.Command = binary.BigEndian.Uint16(data[4:6])
	p.Error = binary.BigEndian.Uint16(data[6:8])
	p.Type = binary.BigEndian.Uint16(data[8:10])
	p.MessageId = uint32(binary.BigEndian.Uint16(data[10:12]))

	payloadLen := int(p.Size)
	available := len(data) - blazeHeaderSize

	if payloadLen > available {
		logger.Warn("BLAZE payload truncated: header says %d bytes, only %d available", payloadLen, available,)
		payloadLen = available
	}

	if payloadLen < 0 {
		payloadLen = 0
	}

	p.Payload = make([]byte, payloadLen)
	copy(p.Payload, data[blazeHeaderSize:blazeHeaderSize+payloadLen])

	logger.Packet("RECEIVED", p.Component, p.Command, p.Type, p.MessageId, p.Payload,)

	return p
}

func EncodePacket(component uint16, command uint16, packetType uint16, messageID uint32, payload []byte,) []byte {
	if len(payload) > 0xFFFF {
		logger.Error(
			"Blaze payload exceeds uint16 size: %d bytes",
			len(payload),
		)
		panic("Blaze payload exceeds uint16 size")
	}

	if messageID > 0xFFFF {
		logger.Error(
			"Blaze message ID exceeds uint16: %d",
			messageID,
		)
		panic("Blaze message ID exceeds uint16")
	}

	data := make([]byte, blazeHeaderSize+len(payload))

	binary.BigEndian.PutUint16(data[0:2], uint16(len(payload)),)
	binary.BigEndian.PutUint16(data[2:4], component,)
	binary.BigEndian.PutUint16(data[4:6], command,)
	binary.BigEndian.PutUint16(data[6:8], 0,)
	binary.BigEndian.PutUint16(data[8:10], packetType,)
	binary.BigEndian.PutUint16(data[10:12], uint16(messageID),)

	copy(data[blazeHeaderSize:], payload)

	logger.Packet("SENT", component, command, packetType, messageID, payload,)

	return data
}
