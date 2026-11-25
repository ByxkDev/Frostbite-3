package redirector

import (
	"bf4/blaze"
)

const (
	RedirectorComponent uint16 = 5
	GetServerInstance   uint16 = 1
)

const (
	GameServerIP          = ""
	GameServerHost        = ""
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
			"Welcome To Syntax",
			"@Byxk",
		},
		Nmap: []emptyStruct{},
		Secu: false,
		Xdns: 0,
	}

	payload, err := encoder.Encode(&response)
	if err != nil {
		return nil
	}

	packet := blaze.EncodePacket(RedirectorComponent, GetServerInstance, blaze.PacketTypeResponse, messageID, payload)
	return packet
}
