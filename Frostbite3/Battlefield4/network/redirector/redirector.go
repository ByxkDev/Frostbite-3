package redirector

import (
	"bytes"
	"encoding/binary"

	"bf4/blaze"
	"bf4/logger"
)

const (
	RedirectorComponent uint16 = 5
	GetServerInstance   uint16 = 1
)

const (
	GameServerIP   = "0.0.0.0"
	GameServerPort = uint32(33152)
)

const (
	tdfInt    byte = 0x00
	tdfString byte = 0x01
	tdfStruct byte = 0x03
	tdfUnion  byte = 0x06
)

func BuildGetServerInstanceResponse(messageID uint32, clientType string) []byte {
	logger.Info("REDIRECTOR: building GetServerInstance response (MessageId=%d)", messageID)
	logger.Debug("REDIRECTOR: Client Type : %q", clientType)
	logger.Debug("REDIRECTOR: Message ID  : %d", messageID)
	logger.Debug("REDIRECTOR: Server IP   : %s", GameServerIP)
	logger.Debug("REDIRECTOR: Server Port : %d", GameServerPort)
	logger.Debug("REDIRECTOR: Secure      : false")

	payload := bytes.NewBuffer(nil)

	// ADDR union
	logger.Trace("REDIRECTOR: writing ADDR union")
	writeTag(payload, "ADDR")
	payload.WriteByte(tdfUnion)
	payload.WriteByte(0x00)

	// VALU struct
	logger.Trace("REDIRECTOR: writing VALU struct")
	writeTag(payload, "VALU")
	payload.WriteByte(tdfStruct)

	// HOST string
	writeString(payload, "HOST", GameServerIP)

	// IP int
	ip := blaze.IPToUInt(GameServerIP)
	logger.Trace("REDIRECTOR: %s -> uint %d (0x%08X)", GameServerIP, ip, ip)
	writeInt(payload, "IP  ", ip)

	// PORT int
	writeInt(payload, "PORT", GameServerPort)

	// End ADDR union
	logger.Trace("REDIRECTOR: closing ADDR union")
	payload.WriteByte(0x00)

	// SECU int
	writeInt(payload, "SECU", 0)

	// XDNS int
	writeInt(payload, "XDNS", 0)

	//logger.Hex(logger.LevelDebug, "REDIRECTOR PAYLOAD RAW", payload.Bytes())

	blaze.DebugTDF("GetServerInstanceResponse", payload.Bytes())

	packet := blaze.EncodePacket(RedirectorComponent, GetServerInstance, blaze.PacketTypeResponse, messageID, payload.Bytes(),)

    //logger.Packet("TX", RedirectorComponent, GetServerInstance, blaze.PacketTypeResponse, messageID, payload.Bytes())
	logger.Response(packet)
	logger.Info("REDIRECTOR: response ready (%d bytes)", len(packet))

	return packet
}

func writeTag(buf *bytes.Buffer, tag string) {
	if len(tag) != 4 {
		logger.Error("REDIRECTOR: invalid TDF tag %q (len=%d), must be exactly 4 characters", tag, len(tag))
		panic("Blaze TDF tags must be exactly 4 characters")
	}

	var value uint32

	for i := 0; i < 4; i++ {
		value = (value << 6) | uint32(byte(tag[i])-0x20)
	}

	encoded := []byte{
		byte(value >> 16),
		byte(value >> 8),
		byte(value),
	}

	logger.Trace("REDIRECTOR: tag %q -> % X", tag, encoded)

	buf.Write(encoded)
}

func writeString(buf *bytes.Buffer, tag string, value string) {
	logger.Trace("REDIRECTOR: writeString tag=%q value=%q", tag, value)

	writeTag(buf, tag)
	buf.WriteByte(tdfString)
	writeTDFInteger(buf, int64(len(value)+1))
	buf.WriteString(value)
	buf.WriteByte(0x00)
}

func writeInt(buf *bytes.Buffer, tag string, value uint32) {
	logger.Trace("REDIRECTOR: writeInt tag=%q value=%d", tag, value)

	writeTag(buf, tag)
	buf.WriteByte(tdfInt)
	writeTDFInteger(buf, int64(value))
}

func writeTDFInteger(buf *bytes.Buffer, value int64) {
	if value < 0 {
		logger.Error("REDIRECTOR: negative Blaze integer %d is not supported", value)
		panic("negative Blaze integer is not supported")
	}

	start := buf.Len()
	original := value

	defer func() {
		logger.Trace("REDIRECTOR: TDF integer %d -> % X", original, buf.Bytes()[start:])
	}()

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
		logger.Error("REDIRECTOR: Blaze payload too large (%d bytes)", len(payload))
		panic("Blaze payload too large")
	}

	var size uint16
	binary.BigEndian.PutUint16([]byte{
		byte(size >> 8),
		byte(size),
	}, uint16(len(payload)))

	logger.Trace("REDIRECTOR: encoded packet size %d", len(payload))

	return uint16(len(payload))
}
