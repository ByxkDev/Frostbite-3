package components

import (
	"bytes"
	"encoding/binary"
	"hash/fnv"
	"sync"
	"time"

	"bf4/blaze"
	"bf4/logger"
	"bf4/network/redirector"
	"bf4/utilities"
)

const (
	Authentication        uint16 = 1
	Redirector            uint16 = 5
	Util                  uint16 = 9
	ConsoleAuthentication uint16 = 35
)

type component35Command10Request struct {
	AUTH string `tdf:"AUTH"`
	EXTB []byte `tdf:"EXTB"`
	EXTI uint64 `tdf:"EXTI"`
}

type personaDetails struct {
	DSNM string `tdf:"DSNM"`
	PID  uint64 `tdf:"PID"` 
	PLAT uint64 `tdf:"PLAT"`
}

type userSessionLoginInfo struct {
	BUID uint64         `tdf:"BUID"` 
	FRSC bool           `tdf:"FRSC"` 
	FRST bool           `tdf:"FRST"`
	KEY  string         `tdf:"KEY"` 
	LLOG uint64         `tdf:"LLOG"` 
	MAIL string         `tdf:"MAIL"`
	PDTL personaDetails `tdf:"PDTL"`
	UID  uint64         `tdf:"UID"`
}

type getUserSessionFromAuthResponse struct {
	ANON bool                 `tdf:"ANON"`
	NTOS bool                 `tdf:"NTOS"`
	SESS userSessionLoginInfo `tdf:"SESS"`
	SPAM bool                 `tdf:"SPAM"`
	UNDR bool                 `tdf:"UNDR"`
}

type userSessionExtendedDataNotify struct {
	ALOC uint64              `tdf:"ALOC"`
	BUID uint64              `tdf:"BUID"`
	CGID blaze.BlazeObjectID `tdf:"CGID"`
	DSNM string              `tdf:"DSNM"`
	FRSC bool                `tdf:"FRSC"`
	FRST bool                `tdf:"FRST"`
	KEY  string              `tdf:"KEY"`
	LAST uint64              `tdf:"LAST"`
	LLOG uint64              `tdf:"LLOG"`
	MAIL string              `tdf:"MAIL"`
	PID  uint64              `tdf:"PID"`
	PLAT uint64              `tdf:"PLAT"`
	UID  uint64              `tdf:"UID"`
	USTP uint64              `tdf:"USTP"`
	XREF uint64              `tdf:"XREF"`
}

type networkAddressUnion struct {
	blaze.Union
	VALU *ipPairAddress `tdfunion:"2"`
}

type qosData struct {
	DBPS uint64 `tdf:"DBPS"`
	NATT uint64 `tdf:"NATT"`
	UBPS uint64 `tdf:"UBPS"`
}

type userExtDataHead struct {
	ADDR networkAddressUnion `tdf:"ADDR"`
	BPS  string              `tdf:"BPS"`
	CTY  string              `tdf:"CTY"`
}

type userExtDataTail struct {
	HWFG uint64                `tdf:"HWFG"`
	QDAT qosData               `tdf:"QDAT"`
	UATT uint64                `tdf:"UATT"`
	ULST []blaze.BlazeObjectID `tdf:"ULST"`
}

type userIdentification struct {
	AID  uint64 `tdf:"AID"`
	ALOC uint64 `tdf:"ALOC"`
	ID   uint64 `tdf:"ID"`
	NAME string `tdf:"NAME"`
	ORIG uint64 `tdf:"ORIG"`
	PIDI uint64 `tdf:"PIDI"`
}

type userAddedNotifyUser struct {
	USER userIdentification `tdf:"USER"`
}

type userUpdatedNotify struct {
	FLGS uint64 `tdf:"FLGS"`
	ID   uint64 `tdf:"ID"`
}

const (
	blazeTypeReply        uint16 = 0x1000
	blazeTypeNotification uint16 = 0x2000

	userSessionsComponent uint16 = 0x7802

	blazePlatformPS3 uint64 = 2

	legacySessionKey = "SessionKey_1337"
	localeEnUS       = 1701729619 
	userAttributes   = 20409902694400
)

var (
	component35TdfFactory = blaze.NewTdfFactory()
	component35TdfEncoder = component35TdfFactory.CreateEncoder(false)
	component35TdfDecoder = component35TdfFactory.CreateDecoder(false)

	tdfEmptyCVAR = func() []byte {
		m := blaze.MustTdfMember("CVAR")
		return append(m.Bytes[:], byte(blaze.TdfTypeVariable), 0x00)
	}()

	component35Mu        sync.RWMutex
	component35OnlineID  string
	component35Auth      string
	component35Exti      uint64
	component35Extb      []byte
	component35PersonaID uint64
)

func HandleComponent35(p blaze.Packet) []byte {
	defer logger.Timed("COMP35")()

	logger.Section(logger.LevelInfo, "COMPONENT 35")
	logger.Info("COMP35: Component=%d Command=%d MessageId=%d Payload=%d", p.Component, p.Command, p.MessageId, len(p.Payload))
	logger.HexDump(logger.LevelDebug, "COMP35 REQUEST DUMP", p.Payload)

	switch p.Command {
	case 10:
		logger.Debug("COMP35: dispatching to command 10 (getUserSessionFromAuth)")
		resp := handleComponent35Command10(p)
		if len(resp) == 0 {
			logger.Warn("COMP35: command 10 produced NO response (MessageId=%d)", p.MessageId)
		}
		return resp
	default:
		logger.Warn("COMP35: UNKNOWN COMMAND=%d MessageId=%d Payload=%d", p.Command, p.MessageId, len(p.Payload))
		logger.HexDump(logger.LevelWarn, "COMP35 UNKNOWN PAYLOAD DUMP", p.Payload)
		return nil
	}
}

func handleComponent35Command10(p blaze.Packet) []byte {
	defer logger.Timed("COMP35/10")()

	var req component35Command10Request
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Error("COMP35/10: TDF decode failed: %v", err)
		logger.HexDump(logger.LevelError, "COMP35/10 DECODE FAILED DUMP", p.Payload)
		return nil
	}

	onlineID := extractConsoleOnlineID(req.EXTB)
	if onlineID == "" {
		onlineID = "Player"
		logger.Warn("COMP35/10: no OnlineId in EXTB, using %q", onlineID)
	}
	if len(req.EXTB) > 0 {
		logConsoleExternalBlob(req.EXTB)
	}

	personaID := personaIDFromOnlineID(onlineID)
	email := onlineID + "@ps3.local"
	now := uint64(time.Now().Unix())
	connGroup := connectionGroup(personaID)

	component35Mu.Lock()
	component35OnlineID = onlineID
	component35Auth = req.AUTH
	component35Exti = req.EXTI
	component35Extb = append(component35Extb[:0], req.EXTB...)
	component35PersonaID = personaID
	component35Mu.Unlock()

	sessionKey := sessionKeyFor(personaID, onlineID)
	logger.Info("COMP35/10: login OnlineId=%q PersonaId=%d EXTI=%d AUTH.len=%d SessionKey=%q", onlineID, personaID, req.EXTI, len(req.AUTH), sessionKey)

	replyPayload, err := component35TdfEncoder.Encode(&getUserSessionFromAuthResponse{
		ANON: false,
		NTOS: false,
		SESS: userSessionLoginInfo{
			BUID: personaID,
			FRSC: false,
			FRST: false,
			KEY:  sessionKey,
			LLOG: now,
			MAIL: email,
			PDTL: personaDetails{DSNM: onlineID, PID: personaID, PLAT: blazePlatformPS3},
			UID:  personaID,
		},
		SPAM: true,
		UNDR: false,
	})
	if err != nil {
		logger.Error("COMP35/10: encode reply failed: %v", err)
		return nil
	}

	extPayload, err := component35TdfEncoder.Encode(&userSessionExtendedDataNotify{
		ALOC: now,
		BUID: personaID,
		CGID: connGroup,
		DSNM: onlineID,
		KEY:  sessionKey,
		LAST: now,
		LLOG: now,
		MAIL: email,
		PID:  personaID,
		PLAT: blazePlatformPS3,
		UID:  personaID,
	})
	if err != nil {
		logger.Error("COMP35/10: encode 7802/0008 failed: %v", err)
		return nil
	}

	addedPayload, err := encodeUserAdded(personaID, onlineID)
	if err != nil {
		logger.Error("COMP35/10: encode 7802/0002 failed: %v", err)
		return nil
	}

	updatedPayload, err := component35TdfEncoder.Encode(&userUpdatedNotify{FLGS: 3, ID: personaID})
	if err != nil {
		logger.Error("COMP35/10: encode 7802/0005 failed: %v", err)
		return nil
	}

	reply := encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), replyPayload)
	notify1 := encodeBlazePacket(userSessionsComponent, 0x0008, 0, blazeTypeNotification, 0, extPayload)
	notify2 := encodeBlazePacket(userSessionsComponent, 0x0002, 0, blazeTypeNotification, 0, addedPayload)
	notify3 := encodeBlazePacket(userSessionsComponent, 0x0005, 0, blazeTypeNotification, 0, updatedPayload)

	logger.HexDump(logger.LevelDebug, "COMP35/10 REPLY", reply)
	logger.HexDump(logger.LevelDebug, "COMP35/10 NOTIFY 7802/0008", notify1)
	logger.HexDump(logger.LevelDebug, "COMP35/10 NOTIFY 7802/0002", notify2)
	logger.HexDump(logger.LevelDebug, "COMP35/10 NOTIFY 7802/0005", notify3)

	out := make([]byte, 0, len(reply)+len(notify1)+len(notify2)+len(notify3))
	out = append(out, reply...)
	out = append(out, notify1...)
	out = append(out, notify2...)
	out = append(out, notify3...)

	logger.Info("COMP35/10: sending reply (%d bytes) + 3 notifications, total %d bytes", len(reply), len(out))
	return out
}

func connectionGroup(personaID uint64) blaze.BlazeObjectID {
	return blaze.NewBlazeObjectID(int64(personaID), blaze.NewBlazeObjectType(userSessionsComponent, 2))
}

func encodeUserExtendedData(addr networkAddressUnion, personaID uint64) ([]byte, error) {
	head, err := component35TdfEncoder.Encode(&userExtDataHead{ADDR: addr})
	if err != nil {
		return nil, err
	}

	tail, err := component35TdfEncoder.Encode(&userExtDataTail{
		UATT: currentUserAttributes(),
		ULST: []blaze.BlazeObjectID{connectionGroup(personaID)},
	})
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	dataTag := blaze.MustTdfMember("DATA")
	buf.Write(dataTag.Bytes[:])
	buf.WriteByte(byte(blaze.TdfTypeStruct))
	buf.Write(head)
	buf.Write(tdfEmptyCVAR)
	buf.Write(tail)
	buf.WriteByte(0x00)
	return buf.Bytes(), nil
}

func encodeUserAdded(personaID uint64, name string) ([]byte, error) {
	addr := networkAddressUnion{Union: blaze.Union{ActiveMember: blaze.UnionUnsetMember}}
	data, err := encodeUserExtendedData(addr, personaID)
	if err != nil {
		return nil, err
	}

	user, err := component35TdfEncoder.Encode(&userAddedNotifyUser{
		USER: userIdentification{
			AID:  personaID,
			ALOC: localeEnUS,
			ID:   personaID,
			NAME: name,
			ORIG: personaID,
			PIDI: personaID,
		},
	})
	if err != nil {
		return nil, err
	}

	return append(data, user...), nil
}

func personaIDFromOnlineID(name string) uint64 {
	h := fnv.New32a()
	h.Write([]byte(name))
	id := uint64(h.Sum32() & 0x3FFFFFFF)
	if id == 0 {
		id = 1
	}
	return id
}

const blazeTypeExtendedLength uint16 = 0x0010

func encodeBlazePacket(component, command, errCode, typ uint16, msgID uint32, payload []byte) []byte {
	n := len(payload)
	extended := n > 0xFFFF
	hdr := 12
	if extended {
		hdr = 14
		typ |= blazeTypeExtendedLength
	}

	out := make([]byte, hdr, hdr+n)
	binary.BigEndian.PutUint16(out[0:], uint16(n))
	binary.BigEndian.PutUint16(out[2:], component)
	binary.BigEndian.PutUint16(out[4:], command)
	binary.BigEndian.PutUint16(out[6:], errCode)
	binary.BigEndian.PutUint16(out[8:], typ)
	binary.BigEndian.PutUint16(out[10:], uint16(msgID))
	if extended {
		binary.BigEndian.PutUint16(out[12:], uint16(n>>16))
	}
	return append(out, payload...)
}

func extractConsoleOnlineID(b []byte) string {
	if len(b) == 0 {
		return ""
	}

	end := bytes.IndexByte(b, 0)
	if end < 0 {
		end = len(b)
	}

	if end == 0 {
		return ""
	}

	for i := 0; i < end; i++ {
		if b[i] < 32 || b[i] > 126 {
			return ""
		}
	}

	return string(b[:end])
}

func logConsoleExternalBlob(b []byte) {
	if len(b) == 0 {
		logger.Debug("COMP35/10: EXTB is empty")
		return
	}

	logger.Debug("COMP35/10: EXTB decoded size=%d", len(b))

	if end := bytes.IndexByte(b, 0); end >= 0 {
		logger.Debug("COMP35/10: EXTB first NUL at offset %d (tail after NUL=%d bytes)", end, len(b)-end-1)
	} else {
		logger.Debug("COMP35/10: EXTB has no NUL terminator")
	}

	onlineID := extractConsoleOnlineID(b)
	if onlineID != "" {
		logger.Info("COMP35/10: EXTB OnlineId=%q", onlineID)
	} else {
		logger.Debug("COMP35/10: EXTB OnlineId could not be extracted")
	}

	strs := logger.PrintableStrings(b, 3)
	logger.Debug("COMP35/10: EXTB printable strings found=%d", len(strs))

	for i, s := range strs {
		logger.Info("COMP35/10: EXTB string[%d]=%q", i, s)
	}

	ascii := make([]byte, 0, len(b))

	for _, v := range b {
		if v >= 32 && v <= 126 {
			ascii = append(ascii, v)
		}
	}

	if len(ascii) > 0 {
		logger.Info("COMP35/10: EXTB ASCII=%q", string(ascii))
	}

	logger.Hex(logger.LevelDebug, "COMP35/10 EXTB RAW", b)
}

func hex64(v uint64) string {
	const digits = "0123456789ABCDEF"
	if v == 0 {
		return "0"
	}

	var buf [16]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = digits[v&0xF]
		v >>= 4
	}

	return string(buf[i:])
}

func GetConsoleOnlineID() string {
	component35Mu.RLock()
	defer component35Mu.RUnlock()
	return component35OnlineID
}

func GetConsoleAuth() string {
	component35Mu.RLock()
	defer component35Mu.RUnlock()
	return component35Auth
}

func GetConsoleExti() uint64 {
	component35Mu.RLock()
	defer component35Mu.RUnlock()
	return component35Exti
}

func GetConsoleExtb() []byte {
	component35Mu.RLock()
	defer component35Mu.RUnlock()
	return append([]byte(nil), component35Extb...)
}

func componentName(id uint16) string {
	switch id {
	case Authentication:
		return "Authentication"
	case Redirector:
		return "Redirector"
	case Util:
		return "Util"
	case ConsoleAuthentication:
		return "ConsoleAuthentication"
	case AssociationLists:
		return "AssociationLists"
	case UserSessions:
		return "UserSessions"
	case Stats:
		return "Stats"
	case Component0801:
		return "Component0801"
	case Packs:
		return "Packs"
	case Inventory:
		return "Inventory"
	case GameManager:
		return "GameManager"
	default:
		return "Unknown"
	}
}

func logResponse(name string, response []byte) {
	if len(response) == 0 {
		logger.Debug("BLAZE: %s produced no response", name)
		return
	}

	logger.Debug("BLAZE: %s response: %d bytes", name, len(response))
	logger.HexDump(logger.LevelTrace, "BLAZE "+name+" RESPONSE DUMP", response)
}

func HandlePacket(data []byte) []byte {
	defer logger.Timed("BLAZE HandlePacket")()

	logger.Request(data)

	packet := blaze.Parse(data)

	logger.Debug("BLAZE: Component=%d (%s) Command=%d Size=%d Type=0x%04X MessageId=%d", packet.Component, componentName(packet.Component), packet.Command, len(data), packet.Type, packet.MessageId)
	logger.Debug("BLAZE: payload=%d bytes, header+overhead=%d bytes", len(packet.Payload), len(data)-len(packet.Payload))

	var response []byte

	switch packet.Component {
	case Redirector:
		response = HandleRedirector(packet)
	case Authentication:
		response = HandleAuthentication(packet)
	case Util:
		response = HandleUtil(packet)
	case ConsoleAuthentication:
		response = HandleComponent35(packet)
	case AssociationLists:
		response = HandleAssociationLists(packet)
	case UserSessions:
		response = HandleUserSessions(packet)
	case Stats:
		response = HandleStats(packet)
	case Component0801:
		response = HandleComponent0801(packet)
	case Packs:
		response = HandlePacks(packet)
	case Inventory:
		response = HandleInventory(packet)
	case GameManager:
		response = HandleGameManager(packet)
	default:
		logger.Warn("BLAZE: Unknown component: %d command: %d", packet.Component, packet.Command)
		logger.Hex(logger.LevelWarn, "BLAZE RAW", data)
		logger.HexDump(logger.LevelWarn, "BLAZE RAW DUMP", data)
		return nil
	}

	if len(response) > 0 {
		logger.Response(response)
	} else {
		logger.Debug("BLAZE: no response for Component=%d (%s) Command=%d MessageId=%d", packet.Component, componentName(packet.Component), packet.Command, packet.MessageId)
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
		logger.Hex(logger.LevelWarn, "REDIRECTOR RAW", packet.Payload)
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
	logger.Analyze("BLAZE client type payload", payload)
	return ""
}

func HandleAuthentication(packet blaze.Packet) []byte {
	logger.Debug("BLAZE: Authentication Command=%d (0x%04X)", packet.Command, packet.Command)

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
	case 29:
		return handleAuthListUserEntitlements2(packet)
	case 36:
		return handleAuthGetAuthToken(packet)
	case 39:
		return handleAuthGrantEntitlement2(packet)
	default:
		logger.Warn("BLAZE: Unknown Authentication command: %d", packet.Command)
		logger.Hex(logger.LevelWarn, "AUTH RAW", packet.Payload)
		return nil
	}
}

func HandleUtil(packet blaze.Packet) []byte {
	logger.Debug("BLAZE: Util Command=%d", packet.Command)

	switch packet.Command {
	case 1:
		logger.Info("BLAZE: Util FetchClientConfig")
		response := utilities.BuildFetchClientConfigResponse(packet.MessageId, string(packet.Payload))
		logResponse("Util FetchClientConfig", response)
		return response
	case 2:
		logger.Debug("BLAZE: Util Ping")
		response := utilities.BuildPingResponse(packet.MessageId)
		logResponse("Util Ping", response)
		return response
	case 5:
		logger.Info("BLAZE: Util GetTelemetryServer")
		return handleUtilGetTelemetryServer(packet)
	case 7:
		logger.Info("BLAZE: Util PreAuth")
		response := utilities.BuildPreAuthResponse(packet.MessageId)
		logResponse("Util PreAuth", response)
		return response
	case 8:
		logger.Info("BLAZE: Util PostAuth")
		return handleUtilPostAuth(packet)
	case 10:
		return handleUtilUserSettingsLoad(packet)
	case 11:
		return handleUtilUserSettingsSave(packet)
	case 12:
		return handleUtilUserSettingsLoadAll(packet)
	case 22:
		logger.Info("BLAZE: Util SetClientMetrics (no response)")
		return nil
	case 28:
		return handleUtilSetUserMode(packet)
	default:
		logger.Warn("BLAZE: Unknown Util command: %d", packet.Command)
		logger.Hex(logger.LevelWarn, "UTIL RAW", packet.Payload)
		return nil
	}
}
