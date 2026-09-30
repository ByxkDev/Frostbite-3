package components

import (
	"bytes"

	"bf4/blaze"
	"bf4/logger"
	"bf4/network/redirector"
	"bf4/utilities"
)

const (
	Authentication uint16 = 1
	Redirector     uint16 = 5
	Util           uint16 = 9
)

func componentName(id uint16) string {
	switch id {
	case Authentication:
		return "Authentication"
	case Redirector:
		return "Redirector"
	case Util:
		return "Util"
	}
	return "Unknown"
}

func logResponse(name string, response []byte) {
	if len(response) == 0 {
		logger.Debug("BLAZE: %s produced no response", name)
		return
	}

	logger.Debug("BLAZE: %s response: %d bytes", name, len(response))
}

func HandlePacket(data []byte) []byte {
	logger.Request(data)
	packet := blaze.Parse(data)

	logger.Debug("BLAZE: Component=%d (%s) Command=%d Size=%d Type=0x%04X MessageId=%d", packet.Component, componentName(packet.Component), packet.Command, len(data), packet.Type, packet.MessageId,)
	//logger.Hex(logger.LevelDebug, "RX PAYLOAD", packet.Payload)

	var response []byte

	switch packet.Component {
	case 5:
		response = HandleRedirector(packet)
	case 1:
		response = HandleAuthentication(packet)
	case 9:
		response = HandleUtil(packet)

	default:
		logger.Warn("BLAZE: Unknown component: %d command: %d", packet.Component, packet.Command)
		logger.Hex(logger.LevelWarn, "BLAZE RAW", data)
		return nil
	}

	if len(response) > 0 {
		logger.Response(response)
	}

	return response
}

func HandleRedirector(packet blaze.Packet) []byte {
	logger.Debug("BLAZE: Redirector Command=%d", packet.Command)

	switch packet.Command {
	case 1:
		clientType := extractClientType(packet.Payload)
		logger.Info("BLAZE: Redirector GetServerInstance")
		logger.Debug("BLAZE: Client Type: %q", clientType)
		response := redirector.BuildGetServerInstanceResponse(packet.MessageId, clientType)
		logResponse("Redirector GetServerInstance", response)
		return response

	default:
		logger.Warn("BLAZE: Unknown Redirector command: %d", packet.Command)
		return nil
	}
}

func extractClientType(payload []byte) string {
	if bytes.Contains(payload, []byte("warsaw client")) {
		logger.Trace("BLAZE: detected client type \"warsaw client\"")
		return "warsaw client"
	}

	if bytes.Contains(payload, []byte("warsaw server")) {
		logger.Trace("BLAZE: detected client type \"warsaw server\"")
		return "warsaw server"
	}

	logger.Debug("BLAZE: client type not recognized in payload (%d bytes)", len(payload))
	return ""
}

func HandleAuthentication(packet blaze.Packet) []byte {
	logger.Debug("BLAZE: Authentication Command=%d", packet.Command)

	switch packet.Command {
	case 7:
		logger.Info("BLAZE: Authentication PreAuth")
		response := utilities.BuildPreAuthResponse(packet.MessageId)
		logResponse("Authentication PreAuth", response)
		return response
	case 8:
		logger.Info("BLAZE: Authentication PostAuth")
		response := utilities.BuildPreAuthResponse(packet.MessageId)
		logResponse("Authentication PostAuth", response)
		return response
	default:
		logger.Warn("BLAZE: Unknown Authentication command: %d", packet.Command)
		return nil
	}
}

func HandleUtil(packet blaze.Packet) []byte {
	logger.Debug("BLAZE: Util Command=%d", packet.Command)

	switch packet.Command {
	case 1:
		logger.Info("BLAZE: Util FetchClientConfig")
		response := utilities.BuildFetchClientConfigResponse(packet.MessageId, packet.Payload)
		logResponse("Util FetchClientConfig", response)
		return response
	case 2:
		logger.Debug("BLAZE: Util Ping")
		response := utilities.BuildPingResponse(packet.MessageId)
		logResponse("Util Ping", response)
		return response
	case 5:
		logger.Info("BLAZE: Util TelemetryServer (no response)")
		return nil
	case 7:
		logger.Info("BLAZE: Util PreAuth")
		response := utilities.BuildPreAuthResponse(packet.MessageId)
		logResponse("Util PreAuth", response)
		return response
	case 8:
		logger.Info("BLAZE: Util PostAuth")
		response := utilities.BuildPreAuthResponse(packet.MessageId)
		logResponse("Util PostAuth", response)
		return response
	case 22:
		logger.Info("BLAZE: Util SetClientMetrics (no response)")
		return nil
	default:
		logger.Warn("BLAZE: Unknown Util command: %d", packet.Command)
		return nil
	}
}
