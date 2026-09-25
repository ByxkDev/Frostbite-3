package components

import (
	"bytes"
	"fmt"

	"bf4/blaze"
	"bf4/network/redirector"
	"bf4/utilities"
)

const (
	Authentication uint16 = 1
	Redirector     uint16 = 5
	Util           uint16 = 9
)

func HandlePacket(data []byte) []byte {
	packet := blaze.Parse(data)

	fmt.Printf("[BLAZE] Component=%d Command=%d Size=%d Type=0x%04X MessageId=%d\n", packet.Component, packet.Command, len(data), packet.Type, packet.MessageId)

	switch packet.Component {
	case Redirector:
		return HandleRedirector(packet)

	case Authentication:
		return HandleAuthentication(packet)

	case Util:
		return HandleUtil(packet)

	default:
		fmt.Printf("[BLAZE] Unknown component: %d command: %d\n", packet.Component, packet.Command)
		fmt.Printf("[BLAZE] Raw: %x\n", data)
		return nil
	}
}

func HandleRedirector(packet blaze.Packet) []byte {
	fmt.Printf("[BLAZE] Redirector Command=%d\n", packet.Command)

	switch packet.Command {
	case 1:
		clientType := extractClientType(packet.Payload)

		fmt.Println("[BLAZE] Redirector GetServerInstance")
		fmt.Printf("[BLAZE] Client Type: %q\n", clientType)

		response := redirector.BuildGetServerInstanceResponse(packet.MessageId, clientType)
		fmt.Printf("[BLAZE] Redirector response: %d bytes\n", len(response))

		return response

	default:
		fmt.Printf("[BLAZE] Unknown Redirector command: %d\n", packet.Command)
		return nil
	}
}

func extractClientType(payload []byte) string {
	if bytes.Contains(payload, []byte("warsaw client")) {
		return "warsaw client"
	}

	if bytes.Contains(payload, []byte("warsaw server")) {
		return "warsaw server"
	}

	return ""
}

func HandleAuthentication(packet blaze.Packet) []byte {
	fmt.Printf("[BLAZE] Authentication Command=%d\n", packet.Command)

	switch packet.Command {
	case 7:
		fmt.Println("[BLAZE] Authentication PreAuth")
		response := utilities.BuildPreAuthResponse(packet.MessageId)
		fmt.Printf("[BLAZE] Authentication PreAuth response: %d bytes\n", len(response))
		return response

	case 8:
		fmt.Println("[BLAZE] Authentication PostAuth")
		response := utilities.BuildPreAuthResponse(packet.MessageId)
		fmt.Printf("[BLAZE] Authentication PostAuth response: %d bytes\n", len(response))
		return response

	default:
		fmt.Printf("[BLAZE] Unknown Authentication command: %d\n", packet.Command)
		return nil
	}
}

func HandleUtil(packet blaze.Packet) []byte {
	fmt.Printf("[BLAZE] Util Command=%d\n", packet.Command)

	switch packet.Command {
	case 1:
		fmt.Println("[BLAZE] Util FetchClientConfig")
		response := utilities.BuildFetchClientConfigResponse(packet.MessageId, packet.Payload)
		fmt.Printf("[BLAZE] Util FetchClientConfig response: %d bytes\n", len(response))
		return response

	case 2:
		fmt.Println("[BLAZE] Util Ping")
		response := utilities.BuildPingResponse(packet.MessageId)
		fmt.Printf("[BLAZE] Util Ping response: %d bytes\n", len(response))
		return response

	case 5:
		fmt.Println("[BLAZE] Util TelemetryServer")
		return nil

	case 7:
		fmt.Println("[BLAZE] Util PreAuth")
		response := utilities.BuildPreAuthResponse(packet.MessageId)
		fmt.Printf("[BLAZE] Util PreAuth response: %d bytes\n", len(response))
		return response

	case 8:
		fmt.Println("[BLAZE] Util PostAuth")
		response := utilities.BuildPreAuthResponse(packet.MessageId)
		fmt.Printf("[BLAZE] Util PostAuth response: %d bytes\n", len(response))
		return response

	case 22:
		fmt.Println("[BLAZE] Util SetClientMetrics")
		return nil

	default:
		fmt.Printf("[BLAZE] Unknown Util command: %d\n", packet.Command)
		return nil
	}
}
