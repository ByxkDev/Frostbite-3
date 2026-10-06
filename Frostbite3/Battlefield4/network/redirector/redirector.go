package redirector

import (
	"bf4/blaze"
	"bf4/logger"
)

const (
	RedirectorComponent uint16 = 5
	GetServerInstance   uint16 = 1
)

const (
	GameServerIP          = "0.0.0.0"
	GameServerHost        = "0.0.0.0"
	GameServerPort uint32 = 33152
)

var encoder = blaze.NewTdfFactory().CreateEncoder(false)

type serverAddress struct {
	Host string `tdf:"HOST"`
	IP   uint32 `tdf:"IP"`
	Port uint32 `tdf:"PORT"`
}

type networkAddress struct {
	blaze.Union
	Valu *serverAddress `tdfunion:"0"`
}

type emptyStruct struct{}

type serverInstanceInfo struct {
	Addr networkAddress `tdf:"ADDR"`
	Amap []emptyStruct  `tdf:"AMAP"`
	Msgs []string       `tdf:"MSGS"`
	Nmap []emptyStruct  `tdf:"NMAP"`
	Secu bool           `tdf:"SECU"`
	Xdns uint32         `tdf:"XDNS"`
}

func BuildGetServerInstanceResponse(messageID uint32, clientType string) []byte {
	logger.Info("REDIRECTOR: building GetServerInstance response (MessageId=%d)", messageID)
	logger.Debug("REDIRECTOR: Client Type: %q", clientType)
	logger.Debug("REDIRECTOR: Message ID: %d", messageID)
	logger.Debug("REDIRECTOR: Server IP: %s", GameServerIP)
	logger.Debug("REDIRECTOR: Server Port: %d", GameServerPort)
	logger.Debug("REDIRECTOR: Secure: false")

	response := serverInstanceInfo{
		Addr: networkAddress{
			Valu: &serverAddress{
				Host: GameServerHost,
				IP:   blaze.IPToUInt(GameServerIP),
				Port: GameServerPort,
			},
		},
		Amap: []emptyStruct{},
		Msgs: []string{
			"Hello World!",
			"@Byxk",
		},
		Nmap: []emptyStruct{},
		Secu: false,
		Xdns: 0,
	}

	payload, err := encoder.Encode(&response)
	if err != nil {
		logger.Warn("REDIRECTOR: GetServerInstance encode failed: %v", err)
		return nil
	}

	logger.Hex(logger.LevelDebug, "REDIRECTOR GetServerInstance payload", payload)

	packet := blaze.EncodePacket(RedirectorComponent, GetServerInstance, blaze.PacketTypeResponse, messageID, payload)

	logger.Response(packet)
	logger.Info("REDIRECTOR: response ready (%d bytes)", len(packet))

	return packet
}
