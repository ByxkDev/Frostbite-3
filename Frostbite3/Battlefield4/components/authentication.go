package components

import (
	"fmt"
	"bf4/blaze"
)

const Authentication uint16 = 1

func HandlePacket(data []byte) []byte {
    packet := blaze.Parse(data)
	fmt.Printf("[BLAZE] Component=%d Command=%d\n", packet.Component, packet.Command,)

	switch packet.Component {
	case Authentication:
		return HandleAuthentication(packet)
	default:
		fmt.Println("Unknown component:", packet.Component,)
	}

	return nil
}

func HandleAuthentication(packet blaze.Packet) []byte {
	fmt.Println("Authentication command:", packet.Command,)
	return nil
}