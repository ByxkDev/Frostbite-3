package components

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"bf4/blaze"
	"bf4/logger"
)

const (
	AssociationLists uint16 = 0x0019
	UserSessions     uint16 = 0x7802
)

var ServerHost = "0.0.0.0"

const tickerPort = 17502

type pssConfig struct {
	ADRS string `tdf:"ADRS"`
	AMAX uint64 `tdf:"AMAX"`
	OMAX uint64 `tdf:"OMAX"`
	PJID string `tdf:"PJID"`
	PORT uint64 `tdf:"PORT"`
	RPRT uint64 `tdf:"RPRT"`
	TIID uint64 `tdf:"TIID"`
}

type telemetryServer struct {
	ADRS string `tdf:"ADRS"`
	ANON bool   `tdf:"ANON"`
	DISA string `tdf:"DISA"`
	EDCT uint64 `tdf:"EDCT"`
	FILT string `tdf:"FILT"`
	LOC  uint64 `tdf:"LOC"`
	MINR uint64 `tdf:"MINR"`
	NOOK string `tdf:"NOOK"`
	PORT uint64 `tdf:"PORT"`
	SDLY uint64 `tdf:"SDLY"`
	SESS string `tdf:"SESS"`
	SKEY string `tdf:"SKEY"`
	SPCT uint64 `tdf:"SPCT"`
	STIM string `tdf:"STIM"`
	SVNM string `tdf:"SVNM"`
}

type tickerServer struct {
	ADRS string `tdf:"ADRS"`
	PORT uint64 `tdf:"PORT"`
	SKEY string `tdf:"SKEY"`
}

type userOptions struct {
	TMOP uint64 `tdf:"TMOP"`
	UID  uint64 `tdf:"UID"`
}

type postAuthResponse struct {
	PSS  pssConfig       `tdf:"PSS"`
	TELE telemetryServer `tdf:"TELE"`
	TICK tickerServer    `tdf:"TICK"`
	UROP userOptions     `tdf:"UROP"`
}

var telemetrySKEY = mustHexString("5eb9caf7d19cb3ddefcb93afaaff818cbbd8e18b9af6ed9bb6b1e8b0a986c6ceb1e2f4d0a9a6a78eb1baea84d3b3ec8d96a4e0c08183868c98b0e0c089e6c6989ab7c2c9e182eed897e2c2d1a3c7ad99b3e9cab1a3d685cd96f0c6b189c3a68d98b8eed091c3a68d96e5dcd59aa58180")

func telemetry() telemetryServer {
	return telemetryServer{
		ADRS: "gostelemetry.blaze3.ea.com",
		FILT: "-GAME/COMM/EXPD",
		LOC:  localeEnUS,
		PORT: 9988,
		SDLY: 15000,
		SESS: "FWpeWJ6Rrp",
		SKEY: telemetrySKEY,
		SPCT: 75,
		STIM: "Default",
		SVNM: "telemetry-3-common",
	}
}

func handleUtilPostAuth(p blaze.Packet) []byte {
	personaID := currentPersonaID()

	resp := postAuthResponse{
		TELE: telemetry(),
		TICK: tickerServer{
			ADRS: ServerHost,
			PORT: tickerPort,
			SKEY: fmt.Sprintf("%d,%s:%d,battlefield-4-ps3,10,50,50,50,50,0,0", personaID, ServerHost, tickerPort),
		},
		UROP: userOptions{TMOP: 1, UID: personaID},
	}

	logger.Info("UTIL: PostAuth UID=%d ticker=%s:%d", personaID, ServerHost, tickerPort)
	return encodeReply(p, &resp, "Util PostAuth")
}

func handleUtilGetTelemetryServer(p blaze.Packet) []byte {
	t := telemetry()
	logger.Info("UTIL: GetTelemetryServer %s:%d", t.ADRS, t.PORT)
	return encodeReply(p, &t, "Util GetTelemetryServer")
}

type listEntitlementsRequest struct {
	ETAG string   `tdf:"ETAG"`
	GNLS []string `tdf:"GNLS"`
}

type entitlement struct {
	DEVI string `tdf:"DEVI"`
	GDAY string `tdf:"GDAY"`
	GNAM string `tdf:"GNAM"` 
	ID   uint64 `tdf:"ID"`
	ISCO uint64 `tdf:"ISCO"`
	PID  uint64 `tdf:"PID"`
	PJID string `tdf:"PJID"`
	PRCA uint64 `tdf:"PRCA"`
	PRID string `tdf:"PRID"` 
	STAT uint64 `tdf:"STAT"` 
	STRC uint64 `tdf:"STRC"`
	TAG  string `tdf:"TAG"`
	TDAY string `tdf:"TDAY"`
	TYPE uint64 `tdf:"TYPE"`
	UCNT uint64 `tdf:"UCNT"`
	VER  uint64 `tdf:"VER"`
}

type listEntitlementsResponse struct {
	NLST []entitlement `tdf:"NLST"`
}

const entitlementGroupPS3 = "BF4PS3"

var ps3Entitlements = []struct {
	tag  string
	prid string
	typ  uint64
}{
	{"ONLINE_ACCESS", "DR:235663400", 1},
	{"PREMIUM_ACCESS", "", 5},
	{"XPACK0_ACCESS", "", 5},
	{"XPACK1_ACCESS", "", 5},
	{"XPACK2_ACCESS", "", 5},
	{"XPACK3_ACCESS", "", 5},
	{"XPACK4_ACCESS", "", 5},
	{"XPACK5_ACCESS", "", 5},
	{"XPACK6_ACCESS", "", 5},
	{"XPACK7_ACCESS", "", 5},
}

func handleAuthListUserEntitlements2(p blaze.Packet) []byte {
	var req listEntitlementsRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("AUTH: ListUserEntitlements2 decode failed: %v", err)
	}
	logger.Info("AUTH: ListUserEntitlements2 ETAG=%q GNLS=%v", req.ETAG, req.GNLS)

	var resp listEntitlementsResponse
	if req.ETAG == "" {
		for i, e := range ps3Entitlements {
			resp.NLST = append(resp.NLST, entitlement{
				GNAM: entitlementGroupPS3,
				ID:   uint64(i + 1),
				PRCA: 2,
				PRID: e.prid,
				STAT: 1,
				TAG:  e.tag,
				TYPE: e.typ,
			})
		}
		resp.NLST = append(resp.NLST, grantedEntitlements()...)
	}

	return encodeReply(p, &resp, "Auth ListUserEntitlements2")
}

type ipAddress struct {
	IP   uint32 `tdf:"IP"`
	MACI uint64 `tdf:"MACI"`
	PORT uint16 `tdf:"PORT"`
}

type ipPairAddress struct {
	EXIP ipAddress `tdf:"EXIP"`
	INIP ipAddress `tdf:"INIP"`
}

type updateNetworkInfoRequest struct {
	ADDR networkAddressUnion `tdf:"ADDR"`
}

type userSessionExtendedDataUpdate struct {
	USID uint64 `tdf:"USID"`
}

func handleUserSessionsUpdateNetworkInfo(p blaze.Packet) []byte {
	var req updateNetworkInfoRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("USERSESSIONS: UpdateNetworkInfo decode failed: %v", err)
	}

	pair := ipPairAddress{}
	if req.ADDR.VALU != nil {
		pair = *req.ADDR.VALU
	}
	if pair.EXIP.IP == 0 {
		pair.EXIP.IP = pair.INIP.IP
		pair.EXIP.PORT = pair.INIP.PORT
	}
	pair.EXIP.MACI, pair.INIP.MACI = 0, 0

	userStateMu.Lock()
	lastAddress = &pair
	userStateMu.Unlock()

	personaID := currentPersonaID()
	logger.Info("USERSESSIONS: UpdateNetworkInfo INIP=%s:%d EXIP=%s:%d", ipString(pair.INIP.IP), pair.INIP.PORT, ipString(pair.EXIP.IP), pair.EXIP.PORT)

	reply := encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), nil)

	notify := extendedDataUpdateNotify(personaID)
	if notify == nil {
		return reply
	}

	logger.HexDump(logger.LevelDebug, "USERSESSIONS UpdateNetworkInfo REPLY", reply)
	logger.HexDump(logger.LevelDebug, "USERSESSIONS NOTIFY 7802/0001", notify)
	return append(reply, notify...)
}

func HandleAssociationLists(p blaze.Packet) []byte {
	switch p.Command {
	case 0x0006, 0x0007:
		logger.Info("ASSOCLISTS: command %d -> empty reply", p.Command)
		return encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), nil)
	default:
		logger.Warn("ASSOCLISTS: unknown command %d", p.Command)
		logger.Hex(logger.LevelWarn, "ASSOCLISTS RAW", p.Payload)
		return nil
	}
}

type setUserInfoAttributeRequest struct {
	ATTV uint64                `tdf:"ATTV"`
	MASK uint64                `tdf:"MASK"`
	ULST []blaze.BlazeObjectID `tdf:"ULST"`
}

var (
	userStateMu   sync.Mutex
	lastAddress   *ipPairAddress 
	userAttrValue uint64         = userAttributes
)

func currentUserAttributes() uint64 {
	userStateMu.Lock()
	defer userStateMu.Unlock()
	return userAttrValue
}

func handleUserSessionsSetUserInfoAttribute(p blaze.Packet) []byte {
	var req setUserInfoAttributeRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("USERSESSIONS: SetUserInfoAttribute decode failed: %v", err)
	}

	userStateMu.Lock()
	old := userAttrValue
	userAttrValue = (old &^ req.MASK) | (req.ATTV & req.MASK)
	updated := userAttrValue
	userStateMu.Unlock()

	logger.Info("USERSESSIONS: SetUserInfoAttribute MASK=0x%X ATTV=0x%X UATT 0x%X -> 0x%X (users=%d)", req.MASK, req.ATTV, old, updated, len(req.ULST))

	reply := encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), nil)
	if notify := extendedDataUpdateNotify(currentPersonaID()); notify != nil {
		return append(reply, notify...)
	}
	return reply
}

func extendedDataUpdateNotify(personaID uint64) []byte {
	addr := networkAddressUnion{Union: blaze.Union{ActiveMember: blaze.UnionUnsetMember}}
	userStateMu.Lock()
	if lastAddress != nil {
		pair := *lastAddress
		if err := blaze.UnionSetValue(&addr, &pair); err != nil {
			logger.Error("USERSESSIONS: set ADDR union failed: %v", err)
		}
	}
	userStateMu.Unlock()

	data, err := encodeUserExtendedData(addr, personaID)
	if err != nil {
		logger.Error("USERSESSIONS: encode DATA failed: %v", err)
		return nil
	}
	usid, err := component35TdfEncoder.Encode(&userSessionExtendedDataUpdate{USID: personaID})
	if err != nil {
		logger.Error("USERSESSIONS: encode USID failed: %v", err)
		return nil
	}
	return encodeBlazePacket(UserSessions, 0x0001, 0, blazeTypeNotification, 0, append(data, usid...))
}

type getAuthTokenResponse struct {
	AUTH string `tdf:"AUTH"`
}

func handleAuthGetAuthToken(p blaze.Packet) []byte {
	token := "AuthToken_" + sessionKeyFor(currentPersonaID(), GetConsoleOnlineID())
	logger.Info("AUTH: GetAuthToken -> %q", token)
	return encodeReply(p, &getAuthTokenResponse{AUTH: token}, "Auth GetAuthToken")
}

const sessionKeyPrefix = "BF4S_"

func sessionKeyFor(personaID uint64, onlineID string) string {
	return fmt.Sprintf("%s%d_%s", sessionKeyPrefix, personaID, onlineID)
}

func parseSessionKey(key string) (uint64, string, bool) {
	if !strings.HasPrefix(key, sessionKeyPrefix) {
		return 0, "", false
	}
	idPart, name, ok := strings.Cut(strings.TrimPrefix(key, sessionKeyPrefix), "_")
	if !ok || name == "" {
		return 0, "", false
	}
	id, err := strconv.ParseUint(idPart, 10, 64)
	if err != nil || id == 0 || personaIDFromOnlineID(name) != id {
		return 0, "", false
	}
	return id, name, true
}

type resumeSessionRequest struct {
	SKEY string `tdf:"SKEY"`
}

func handleUserSessionsResumeSession(p blaze.Packet) []byte {
	var req resumeSessionRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("USERSESSIONS: ResumeSession decode failed: %v", err)
	}

	reply := encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), nil)

	if personaID, name, ok := parseSessionKey(req.SKEY); ok {
		component35Mu.Lock()
		restored := component35PersonaID != personaID
		component35PersonaID = personaID
		component35OnlineID = name
		component35Mu.Unlock()
		logger.Info("USERSESSIONS: ResumeSession OnlineId=%q PersonaId=%d (restored=%t)", name, personaID, restored)
		return reply
	}

	if req.SKEY == legacySessionKey {
		if currentPersonaID() == 0 {
			logger.Warn("USERSESSIONS: ResumeSession with old key %q after a server restart: user unknown. Quit to the XMB and start BF4 again to log in fresh.", req.SKEY)
		} else {
			logger.Info("USERSESSIONS: ResumeSession with old key, keeping current user %q", GetConsoleOnlineID())
		}
		return reply
	}

	logger.Warn("USERSESSIONS: ResumeSession unknown key %q -> error reply", req.SKEY)
	return encodeBlazePacket(p.Component, p.Command, userSessionsErrSessionNotFound, 0x3000, uint32(p.MessageId), nil)
}

const userSessionsErrSessionNotFound uint16 = 0x0001

func HandleUserSessions(p blaze.Packet) []byte {
	switch p.Command {
	case 0x0023:
		return handleUserSessionsResumeSession(p)
	case 0x0014:
		return handleUserSessionsUpdateNetworkInfo(p)
	case 0x001A:
		return handleUserSessionsSetUserInfoAttribute(p)
	default:
		logger.Warn("USERSESSIONS: unknown command %d (0x%04X)", p.Command, p.Command)
		logger.Hex(logger.LevelWarn, "USERSESSIONS RAW", p.Payload)
		return nil
	}
}

func encodeReply(p blaze.Packet, obj interface{}, name string) []byte {
	payload, err := component35TdfEncoder.Encode(obj)
	if err != nil {
		logger.Error("BLAZE: %s encode failed: %v", name, err)
		return nil
	}
	out := encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), payload)
	logResponse(name, out)
	return out
}

func currentPersonaID() uint64 {
	component35Mu.RLock()
	defer component35Mu.RUnlock()
	return component35PersonaID
}

func ipString(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}

func mustHexString(s string) string {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}