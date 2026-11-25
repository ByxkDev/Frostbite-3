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

type use struct {
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

type setClientMetricsRequest struct {
	UBPS uint64 `tdf:"UBPS"`
	NAME string `tdf:"NAME"`
	NATT uint64 `tdf:"NATT"`
	DBPS uint64 `tdf:"DBPS"`
}

func handleUtilSetClientMetrics(p blaze.Packet) []byte {
	var req setClientMetricsRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("UTIL: SetClientMetrics decode: %v", err)
	}
	logger.Info("UTIL: SetClientMetrics (%d bytes) -> empty reply", len(p.Payload))
	return encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), nil)
}

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
	switch p.Command {
	case 10:
		return handleComponent35Command10(p)
	default:
		return nil
	}
}

func handleComponent35Command10(p blaze.Packet) []byte {
	var req component35Command10Request
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		return nil
	}

	onlineID := extractConsoleOnlineID(req.EXTB)
	if onlineID == "" {
		onlineID = "Player"
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

	rememberUser(personaID, onlineID, req.EXTI)

	sessionKey := sessionKeyFor(personaID, onlineID)

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
		return nil
	}

	extPayload, err := component35TdfEncoder.Encode(&use{
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
		return nil
	}

	addedPayload, err := encodeUserAdded(personaID, onlineID)
	if err != nil {
		return nil
	}

	updatedPayload, err := component35TdfEncoder.Encode(&userUpdatedNotify{FLGS: 3, ID: personaID})
	if err != nil {
		return nil
	}

	reply := encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), replyPayload)
	notify1 := encodeBlazePacket(userSessionsComponent, 0x0008, 0, blazeTypeNotification, 0, extPayload)
	notify2 := encodeBlazePacket(userSessionsComponent, 0x0002, 0, blazeTypeNotification, 0, addedPayload)
	notify3 := encodeBlazePacket(userSessionsComponent, 0x0005, 0, blazeTypeNotification, 0, updatedPayload)

	out := make([]byte, 0, len(reply)+len(notify1)+len(notify2)+len(notify3))
	out = append(out, reply...)
	out = append(out, notify1...)
	out = append(out, notify2...)
	out = append(out, notify3...)

	return out
}

func connectionGroup(personaID uint64) blaze.BlazeObjectID {
	return blaze.NewBlazeObjectID(int64(personaID), blaze.NewBlazeObjectType(userSessionsComponent, 2))
}

func encodeUserExtendedData(addr networkAddressUnion, personaID uint64, extra ...blaze.BlazeObjectID) ([]byte, error) {
	head, err := component35TdfEncoder.Encode(&userExtDataHead{ADDR: addr})
	if err != nil {
		return nil, err
	}

	tail, err := component35TdfEncoder.Encode(&userExtDataTail{
		UATT: currentUserAttributes(),
		ULST: append([]blaze.BlazeObjectID{connectionGroup(personaID)}, extra...),
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

	for i := range end {
		if b[i] < 32 || b[i] > 126 {
			return ""
		}
	}

	return string(b[:end])
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

func HandlePacket(data []byte) []byte {
	packet := blaze.Parse(data)

	switch packet.Component {
	case Redirector:
		return HandleRedirector(packet)
	case Authentication:
		return HandleAuthentication(packet)
	case Util:
		return HandleUtil(packet)
	case ConsoleAuthentication:
		return HandleComponent35(packet)
	case AssociationLists:
		return HandleAssociationLists(packet)
	case UserSessions:
		return HandleUserSessions(packet)
	case Stats:
		return HandleStats(packet)
	case Component0801:
		if packet.Command == component0801Echo {
			return handleComponent0801Echo(packet)
		}
		return HandleComponent0801(packet)
	case Packs:
		return HandlePacks(packet)
	case Inventory:
		return HandleInventory(packet)
	case GameManager:
		return HandleGameManager(packet)
	case GameReporting:
		return HandleGameReporting(packet)
	case PlayerGroups:
		return HandlePlayerGroups(packet)
	case Component0804:
		return HandleComponent0804(packet)
	default:
		return nil
	}
}

const (
	component0801Echo      uint16 = 0x0032
	component0801EchoError uint16 = 0x4002
	blazeTypeErrorReply    uint16 = 0x3000
)

func handleComponent0801Echo(packet blaze.Packet) []byte {
	return encodeBlazePacket(packet.Component, packet.Command, component0801EchoError, blazeTypeErrorReply, uint32(packet.MessageId), nil)
}

func HandleRedirector(packet blaze.Packet) []byte {
	switch packet.Command {
	case 1:
		clientType := extractClientType(packet.Payload)
		return redirector.BuildGetServerInstanceResponse(packet.MessageId, clientType)
	default:
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
	switch packet.Command {
	case 7:
		return utilities.BuildPreAuthResponse(packet.MessageId)
	case 8:
		return utilities.BuildPreAuthResponse(packet.MessageId)
	case 29:
		return handleAuthListUserEntitlements2(packet)
	case 36:
		return handleAuthGetAuthToken(packet)
	case 39:
		return handleAuthGrantEntitlement2(packet)
	case 199:
		return HandleTicketLogin(packet)
	default:
		return nil
	}
}

func HandleUtil(packet blaze.Packet) []byte {
	switch packet.Command {
	case 1:
		return utilities.BuildFetchClientConfigResponse(packet.MessageId, string(packet.Payload))
	case 2:
		return utilities.BuildPingResponse(packet.MessageId)
	case 5:
		return handleUtilGetTelemetryServer(packet)
	case 7:
		return utilities.BuildPreAuthResponse(packet.MessageId)
	case 8:
		return handleUtilPostAuth(packet)
	case 10:
		return handleUtilUserSettingsLoad(packet)
	case 11:
		return handleUtilUserSettingsSave(packet)
	case 12:
		return handleUtilUserSettingsLoadAll(packet)
	case 28:
		return handleUtilSetUserMode(packet)
	case 22:
		return handleUtilSetClientMetrics(packet)
	default:
		return nil
	}
}

func logResponse(name string, response []byte) {
}
