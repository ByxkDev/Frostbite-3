package components

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"bf4/blaze"
	"bf4/logger"
	"bf4/server"
	"bf4/utilities"
)

//   - no MySQL: any host login is accepted (set HostPassword to require one)
//   - games live in the bf4/server registry, so they show up in the PS3's server browser and matchmaking straight away

var HostPassword = ""

var (
	HostInstance = "battlefield-4-ps3"
	HostPlatform = "ps3"
)

type hostSession struct {
	mu         sync.Mutex
	PersonaID  uint64
	Name       string
	Mail       string
	SessionKey string
	Addr       ipPairAddress
	Games      []uint32
}

func (h *hostSession) snapshot() hostSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	return hostSession{PersonaID: h.PersonaID, Name: h.Name, Mail: h.Mail, SessionKey: h.SessionKey, Addr: h.Addr, Games: append([]uint32(nil), h.Games...)}
}

func (h *hostSession) serverAddr() server.IpPairAddress {
	h.mu.Lock()
	defer h.mu.Unlock()
	return server.IpPairAddress{
		EXIP: server.IpAddress{IP: h.Addr.EXIP.IP, PORT: h.Addr.EXIP.PORT},
		INIP: server.IpAddress{IP: h.Addr.INIP.IP, PORT: h.Addr.INIP.PORT},
	}
}

const (
	GameReporting uint16 = 0x001C
	Clubs         uint16 = 0x000B
)

func handleHostPacket(c *Conn, h *hostSession, p blaze.Packet) []byte {
	logger.Debug("HOST %d: Component=%d (%s) Command=%d MessageId=%d", c.ID, p.Component, componentName(p.Component), p.Command, p.MessageId)

	switch p.Component {
	case Util:
		return hostUtil(c, h, p)
	case Authentication:
		return hostAuth(c, h, p)
	case UserSessions:
		return hostUserSessions(c, h, p)
	case GameManager:
		return hostGameManager(c, h, p)
	case GameReporting:
		return hostGameReporting(c, h, p)
	case Stats:
		return HandleStats(p)
	case AssociationLists:
		return HandleAssociationLists(p)
	case Component0801:
		return HandleComponent0801(p)
	case Packs:
		return HandlePacks(p)
	case Inventory:
		return HandleInventory(p)
	case Clubs:
		logger.Info("HOST %d: Clubs command 0x%04X -> empty reply", c.ID, p.Command)
		return emptyReply(p)
	default:
		logger.Warn("HOST %d: unknown component %d command %d", c.ID, p.Component, p.Command)
		logger.Hex(logger.LevelWarn, "HOST RAW", p.Payload)
		return nil
	}
}

func hostNotify(component, command uint16, obj interface{}) []byte {
	payload, err := gmTdfEncoder.Encode(obj)
	if err != nil {
		logger.Error("HOST: encode notification %d/0x%04X failed: %v", component, command, err)
		return nil
	}
	return encodeBlazePacket(component, command, 0, blazeTypeNotification, 0, payload)
}

type qosPingSite struct {
	PSA string `tdf:"PSA"`
	PSP uint16 `tdf:"PSP"`
	SNA string `tdf:"SNA"`
}

type qosConfig struct {
	BWPS qosPingSite            `tdf:"BWPS"`
	LNP  uint32                 `tdf:"LNP"`
	LTPS map[string]qosPingSite `tdf:"LTPS"`
	SVID uint32                 `tdf:"SVID"`
}

type configMap struct {
	CONF map[string]string `tdf:"CONF"`
}

type hostPreAuthResponse struct {
	ANON bool      `tdf:"ANON"`
	ASRC string    `tdf:"ASRC"`
	CIDS []uint32  `tdf:"CIDS"`
	CONF configMap `tdf:"CONF"`
	INST string    `tdf:"INST"`
	MINR bool      `tdf:"MINR"`
	NASP string    `tdf:"NASP"`
	PLAT string    `tdf:"PLAT"`
	QOSS qosConfig `tdf:"QOSS"`
	RSRC string    `tdf:"RSRC"`
	SVER string    `tdf:"SVER"`
}

var hostComponentIDs = []uint32{
	0x0001, 0x0019, 0x0004, 0x001B, 0x001C, 0x0006, 0x0007, 0x0009, 0x000A,
	0x0801, 0x0802, 0x0803, 0x000B, 0x7800, 0x7801, 0x7802, 0x7803, 0x0014,
	0x7805, 0x7806, 0x07D0,
}

const qosPort = 17502

func hostUtil(c *Conn, h *hostSession, p blaze.Packet) []byte {
	switch p.Command {
	case 0x0001:
		var req struct {
			CFID string `tdf:"CFID"`
		}
		_ = gmTdfDecoder.Decode(p.Payload, &req)
		resp := configMap{CONF: map[string]string{}}
		if req.CFID == "GOSAchievements" {
			resp.CONF["Achievements"] = "ACH32_00,ACH33_00,ACH34_00,ACH35_00,ACH36_00,ACH37_00,ACH38_00,ACH39_00,ACH40_00,XPACH01_00,XPACH02_00,XPACH03_00,XPACH04_00,XPACH05_00,XP2ACH01_00,XP2ACH04_00,XP2ACH03_00,XP2ACH05_00,XP2ACH02_00,XP3ACH01_00,XP3ACH05_00,XP3ACH03_00,XP3ACH04_00,XP3ACH02_00,XP4ACH01_00,XP4ACH02_00,XP4ACH03_00,XP4ACH04_00,XP4ACH05_00,XP5ACH01_00,XP5ACH02_00,XP5ACH03_00,XP5ach04_00,XP5ach05_00"
			resp.CONF["WinCodes"] = "r01_00,r05_00,r04_00,r03_00,r02_00,r10_00,r08_00,r07_00,r06_00,r09_00,r11_00,r12_00,r13_00,r14_00,r15_00,r16_00,r17_00,r18_00,r19_00,r20_00,r21_00,r22_00,r23_00,r24_00,r25_00,r26_00,r27_00,r28_00,r29_00,r30_00,r31_00,r32_00,r33_00,r35_00,r36_00,r37_00,r34_00,r38_00,r39_00,r40_00,r41_00,r42_00,r43_00,r44_00,r45_00,xp2rgm_00,xp2rntdmcq_00,xp2rtdmc_00,xp3rts_00,xp3rdom_00,xp3rnts_00,xp3rngm_00,xp4rndom_00,xp4rscav_00,xp4rnscv_00,xp4ramb1_00,xp4ramb2_00,xp5r502_00,xp5r501_00,xp5ras_00,xp5asw_00"
		}
		logger.Info("HOST %d: FetchClientConfig CFID=%q -> %d keys", c.ID, req.CFID, len(resp.CONF))
		return gmReply(p, &resp, "Host FetchClientConfig")

	case 0x0002:
		return utilities.BuildPingResponse(p.MessageId)

	case 0x0005: 
		return handleUtilGetTelemetryServer(p)

	case 0x0007: 
		site := qosPingSite{PSA: ServerHost, PSP: qosPort, SNA: server.PingSite}
		resp := hostPreAuthResponse{
			ASRC: "300294",
			CIDS: hostComponentIDs,
			CONF: configMap{CONF: map[string]string{
				"connIdleTimeout":           "120s",
				"defaultRequestTimeout":     "80s",
				"pingPeriod":                "20s",
				"voipHeadsetUpdateRate":     "1000",
				"xlspConnectionIdleTimeout": "300",
			}},
			INST: HostInstance,
			NASP: "cem_ea_id",
			PLAT: HostPlatform,
			QOSS: qosConfig{BWPS: site, LNP: 10, LTPS: map[string]qosPingSite{server.PingSite: site}, SVID: 1337},
			RSRC: "300294",
			SVER: "Blaze 13.15.08.0 (CL# 9442625)",
		}
		logger.Info("HOST %d: PreAuth (INST=%s PLAT=%s QoS=%s:%d)", c.ID, HostInstance, HostPlatform, ServerHost, qosPort)
		return gmReply(p, &resp, "Host PreAuth")

	case 0x0008: 
		hs := h.snapshot()
		resp := postAuthResponse{
			TELE: telemetry(),
			TICK: tickerServer{
				ADRS: ServerHost,
				PORT: tickerPort,
				SKEY: fmt.Sprintf("%d,%s:%d,battlefield-4-ps3,10,50,50,50,50,0,0", hs.PersonaID, ServerHost, tickerPort),
			},
			UROP: userOptions{TMOP: 1, UID: hs.PersonaID},
		}
		logger.Info("HOST %d: PostAuth UID=%d", c.ID, hs.PersonaID)
		return gmReply(p, &resp, "Host PostAuth")

	case 0x000C: 
		return gmReply(p, &userSettingsLoadAllResponse{SMAP: map[string]string{"cust": "", "sdt": ""}}, "Host UserSettingsLoadAll")

	case 0x000B, 0x0016, 0x0017, 0x001A, 0x001C: 
		logger.Info("HOST %d: Util 0x%04X -> empty reply", c.ID, p.Command)
		return emptyReply(p)

	default:
		logger.Warn("HOST %d: unknown Util command %d (0x%04X)", c.ID, p.Command, p.Command)
		logger.Hex(logger.LevelWarn, "HOST UTIL RAW", p.Payload)
		return nil
	}
}

type hostPersonaDetails struct {
	DSNM string `tdf:"DSNM"`
	LAST uint64 `tdf:"LAST"`
	PID  uint64 `tdf:"PID"`
	PLAT uint64 `tdf:"PLAT"`
	STAS uint32 `tdf:"STAS"` 
	XREF uint64 `tdf:"XREF"`
	XTYP uint32 `tdf:"XTYP"`
}

type hostSessionInfo struct {
	BUID uint64             `tdf:"BUID"`
	FRST bool               `tdf:"FRST"`
	KEY  string             `tdf:"KEY"`
	LLOG uint64             `tdf:"LLOG"`
	MAIL string             `tdf:"MAIL"`
	PDTL hostPersonaDetails `tdf:"PDTL"`
	UID  uint64             `tdf:"UID"`
}

type hostLoginResponse struct {
	AGUP bool            `tdf:"AGUP"`
	ANON bool            `tdf:"ANON"`
	NTOS bool            `tdf:"NTOS"`
	PCTK string          `tdf:"PCTK"`
	SESS hostSessionInfo `tdf:"SESS"`
	SPAM bool            `tdf:"SPAM"`
	UNDR bool            `tdf:"UNDR"`
}

const platformPS3 = 2

func hostAuth(c *Conn, h *hostSession, p blaze.Packet) []byte {
	switch p.Command {
	case 0x0028: 
		var req struct {
			MAIL string `tdf:"MAIL"`
			PASS string `tdf:"PASS"`
		}
		_ = gmTdfDecoder.Decode(p.Payload, &req)

		if HostPassword != "" && req.PASS != HostPassword {
			logger.Warn("HOST %d: Login %q rejected (wrong password)", c.ID, req.MAIL)
			return encodeBlazePacket(p.Component, p.Command, 0x000B, 0x3000, uint32(p.MessageId), nil)
		}

		mail := req.MAIL
		if mail == "" {
			mail = "bf4.server@emu.local"
		}
		name := mail
		if i := strings.IndexByte(name, '@'); i > 0 {
			name = name[:i]
		}
		id := personaIDFromOnlineID("host:" + mail)

		h.mu.Lock()
		h.Mail, h.Name, h.PersonaID = mail, name, id
		h.SessionKey = sessionKeyFor(id, name)
		h.mu.Unlock()

		logger.Info("HOST %d: Login MAIL=%q -> persona %d %q", c.ID, mail, id, name)
		return append(gmReply(p, &hostLoginResponse{
			PCTK: "PlayerTicket_1337",
			SESS: hostSession2Info(h, platformPS3),
		}, "Host Login"), hostUserNotifies(h)...)

	case 0x006E: 
		var req struct {
			PNAM string `tdf:"PNAM"`
		}
		_ = gmTdfDecoder.Decode(p.Payload, &req)
		if req.PNAM != "" {
			h.mu.Lock()
			h.Name = req.PNAM
			h.mu.Unlock()
		}
		logger.Info("HOST %d: LoginPersona %q", c.ID, req.PNAM)
		return append(gmReply(p, ptr(hostSession2Info(h, platformsPS3)), "Host LoginPersona"), hostUserNotifies(h)...)

	case 0x001D:
		return handleAuthListUserEntitlements2(p)
	case 0x0024:
		return handleAuthGetAuthToken(p)

	default:
		logger.Warn("HOST %d: unknown Authentication command %d (0x%04X)", c.ID, p.Command, p.Command)
		logger.Hex(logger.LevelWarn, "HOST AUTH RAW", p.Payload)
		return nil
	}
}

func ptr[T any](v T) *T { return &v }

func hostSession2Info(h *hostSession, plat uint64) hostSessionInfo {
	hs := h.snapshot()
	now := uint64(time.Now().Unix())
	return hostSessionInfo{
		BUID: hs.PersonaID,
		KEY:  hs.SessionKey,
		LLOG: now,
		MAIL: hs.Mail,
		PDTL: hostPersonaDetails{DSNM: hs.Name, LAST: now, PID: hs.PersonaID, PLAT: plat, STAS: 2},
		UID:  hs.PersonaID,
	}
}

func hostUserNotifies(h *hostSession) []byte {
	hs := h.snapshot()
	now := uint64(time.Now().Unix())
	out := hostNotify(UserSessions, 0x0008, &userSessionExtendedDataNotify{
		ALOC: now, BUID: hs.PersonaID, CGID: connectionGroup(hs.PersonaID), DSNM: hs.Name,
		KEY: hs.SessionKey, LAST: now, LLOG: now, MAIL: hs.Mail, PID: hs.PersonaID,
		PLAT: platformPS3, UID: hs.PersonaID,
	})
	out = append(out, userAddedPacket(hs.PersonaID, hs.Name)...)
	out = append(out, hostNotify(UserSessions, 0x0005, &userUpdatedNotify{FLGS: 3, ID: hs.PersonaID})...)
	return out
}

func userAddedPacket(personaID uint64, name string) []byte {
	payload, err := encodeUserAdded(personaID, name)
	if err != nil {
		logger.Error("USERSESSIONS: encode UserAdded failed: %v", err)
		return nil
	}
	return encodeBlazePacket(UserSessions, 0x0002, 0, blazeTypeNotification, 0, payload)
}

func hostUserSessions(c *Conn, h *hostSession, p blaze.Packet) []byte {
	switch p.Command {
	case 0x0014:  
		var req updateNetworkInfoRequest
		_ = gmTdfDecoder.Decode(p.Payload, &req)

		pair := ipPairAddress{}
		if req.ADDR.VALU != nil {
			pair = *req.ADDR.VALU
		}

		if ip := c.RemoteIP(); ip != 0 {
			pair.EXIP.IP = ip
		}
		if pair.EXIP.PORT == 0 {
			pair.EXIP.PORT = pair.INIP.PORT
		}
		if pair.INIP.IP == 0 {
			pair.INIP = pair.EXIP
		}
		pair.EXIP.MACI, pair.INIP.MACI = 0, 0

		h.mu.Lock()
		h.Addr = pair
		id := h.PersonaID
		h.mu.Unlock()

		logger.Info("HOST %d: UpdateNetworkInfo EXIP=%s:%d INIP=%s:%d", c.ID, ipString(pair.EXIP.IP), pair.EXIP.PORT, ipString(pair.INIP.IP), pair.INIP.PORT)

		addr := networkAddressUnion{}
		_ = blaze.UnionSetValue(&addr, &pair)
		data, err := encodeUserExtendedData(addr, id)
		if err != nil {
			return emptyReply(p)
		}
		usid, _ := component35TdfEncoder.Encode(&userSessionExtendedDataUpdate{USID: id})
		return append(emptyReply(p), encodeBlazePacket(UserSessions, 0x0001, 0, blazeTypeNotification, 0, append(data, usid...))...)

	case 0x0023:
		logger.Info("HOST %d: ResumeSession -> empty reply", c.ID)
		return emptyReply(p)

	default:
		logger.Info("HOST %d: UserSessions 0x%04X -> empty reply", c.ID, p.Command)
		return emptyReply(p)
	}
}

func hostGameManager(c *Conn, h *hostSession, p blaze.Packet) []byte {
	switch p.Command {
	case 0x0001:
		var req server.BF4CreateGameRequest
		if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
			logger.Debug("HOST %d: createGame decode stopped early: %v", c.ID, err)
		}
		hs := h.snapshot()
		g := server.Games.CreateFromBF4(req, hs.PersonaID, hs.Name, h.serverAddr(), c.Push)

		h.mu.Lock()
		h.Games = append(h.Games, g.ID)
		h.mu.Unlock()

		logger.Info("HOST %d: createGame GID=%d %q level=%q mode=%q max=%d host=%s:%d",
			c.ID, g.ID, g.Name, g.Level, g.Mode, g.MaxPlayers, ipString(g.HostIP), g.HostPort)

		reply := gmReply(p, &server.CreateGameResponse{GID: g.ID}, "Host createGame")
		return append(reply, gmNotify(notifyGameSetup, &server.NotifyGameSetupBF4{
			GAME: server.Games.BF4Data(g, nil),
			PROS: []server.BF4Player{},
		})...)

	case 0x000F: 
		var req server.GameIDRequest
		_ = gmTdfDecoder.Decode(p.Payload, &req)
		gid := firstGame(h, req.GID)
		logger.Info("HOST %d: finalizeGameCreation GID=%d", c.ID, gid)
		return append(emptyReply(p), gmNotify(notifyPlatformHostInit, &server.NotifyPlatformHostInitialized{
			GID: gid, PHID: h.snapshot().PersonaID,
		})...)

	case 0x0007:
		var req server.SetGameAttributesRequest
		_ = gmTdfDecoder.Decode(p.Payload, &req)
		gid := firstGame(h, req.GID)
		server.Games.SetAttributes(gid, req.ATTR)
		logger.Info("HOST %d: setGameAttributes GID=%d %v", c.ID, gid, req.ATTR)
		return append(emptyReply(p), gmNotify(notifyGameAttribChange, &server.NotifyGameAttribChange{ATTR: req.ATTR, GID: gid})...)

	case 0x0004: 
		var req server.SetGameSettingsRequest
		_ = gmTdfDecoder.Decode(p.Payload, &req)
		gid := firstGame(h, req.GID)
		server.Games.SetSettings(gid, req.GSET)
		logger.Info("HOST %d: setGameSettings GID=%d GSET=0x%X", c.ID, gid, req.GSET)
		return append(emptyReply(p), gmNotify(notifyGameSettingsChange, &server.NotifyGameSettingsChange{ATTR: req.GSET, GID: gid})...)

	case 0x0005:
		var req server.SetPlayerCapacityRequest
		_ = gmTdfDecoder.Decode(p.Payload, &req)
		gid := firstGame(h, req.GID)
		if len(req.PCAP) > 0 {
			server.Games.SetCapacity(gid, req.PCAP[0])
		}
		logger.Info("HOST %d: setPlayerCapacity GID=%d %v", c.ID, gid, req.PCAP)
		return emptyReply(p)

	case 0x0027: 
		var req server.UpdateGameNameRequest
		_ = gmTdfDecoder.Decode(p.Payload, &req)
		gid := firstGame(h, req.GID)
		server.Games.SetName(gid, req.GNAM)
		logger.Info("HOST %d: updateGameName GID=%d %q", c.ID, gid, req.GNAM)
		return emptyReply(p)

	case 0x0029: 
		var req server.SetGameModRegisterRequest
		_ = gmTdfDecoder.Decode(p.Payload, &req)
		gid := firstGame(h, req.GID)
		server.Games.SetModRegister(gid, req.GMRG)
		logger.Info("HOST %d: setGameModRegister GID=%d GMRG=%d", c.ID, gid, req.GMRG)
		return append(emptyReply(p), gmNotify(notifyGameModRegister, &server.NotifyGameModRegisterChanged{GMID: gid, GMRG: req.GMRG})...)

	case 0x0013: 
		gid := firstGame(h, 0)
		server.Games.SetState(gid, server.GameStatePreGame)
		logger.Info("HOST %d: replayGame GID=%d -> PRE_GAME", c.ID, gid)
		return append(emptyReply(p), gmNotify(notifyGameStateChange, &server.NotifyGameStateChange{GID: gid, GSTA: server.GameStatePreGame})...)

	case 0x0070: 
		logger.Info("HOST %d: swapPlayersTeam -> empty reply", c.ID)
		return emptyReply(p)

	case 0x001D: 
		return hostUpdateMeshConnection(c, h, p)

	default:
		return HandleGameManager(p)
	}
}

const (
	notifyJoiningPlayerInitConns uint16 = 0x0016 
	notifyPlayerClaimingReserv   uint16 = 0x0019 
	notifyPlayerJoinCompleted    uint16 = 0x001E 
	notifyPlatformHostInit       uint16 = 0x0047
	notifyGameAttribChange       uint16 = 0x0050 
	notifyGameSettingsChange     uint16 = 0x006E 
	notifyGamePlayerStateChange  uint16 = 0x0074 
	notifyGameModRegister        uint16 = 0x007B 

	playerStateConnected uint32 = 4
	removeReasonLeft     uint32 = 1
)

func firstGame(h *hostSession, gid uint32) uint32 {
	if gid != 0 {
		return gid
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.Games) > 0 {
		return h.Games[len(h.Games)-1]
	}
	return 0
}

func hostUpdateMeshConnection(c *Conn, h *hostSession, p blaze.Packet) []byte {
	var req server.UpdateMeshConnectionRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("HOST %d: updateMeshConnection decode stopped early: %v", c.ID, err)
	}
	gid := firstGame(h, req.GID)
	pid := uint64(req.TCG.ID)
	reply := emptyReply(p)

	if req.STAT == 2 {
		server.Games.SetPlayerState(gid, pid, playerStateConnected)
		logger.Info("HOST %d: updateMeshConnection GID=%d player %d CONNECTED to the host", c.ID, gid, pid)

		out := append(reply, extendedDataUpdateNotify(pid)...)
		out = append(out, gmNotify(notifyGamePlayerStateChange, &server.NotifyGamePlayerStateChange{GID: gid, PID: pid, STAT: playerStateConnected})...)
		return append(out, gmNotify(notifyPlayerJoinCompleted, &server.NotifyPlayerJoinCompleted{GID: gid, PID: pid})...)
	}

	left := server.Games.Leave(gid, pid)
	logger.Info("HOST %d: updateMeshConnection GID=%d player %d STAT=%d -> removed (was in game: %t)", c.ID, gid, pid, req.STAT, left)
	removed := gmNotify(notifyPlayerRemoved, &server.NotifyPlayerRemoved{GID: gid, PID: pid, REAS: removeReasonLeft})

	if pid == currentPersonaID() {
		if push := currentPush(); push != nil {
			_ = push(removed)
		}
	}
	return append(reply, removed...)
}

type gameReportResultNotify struct {
	FNL  bool   `tdf:"FNL"`
	GHID uint64 `tdf:"GHID"`
	GRID uint64 `tdf:"GRID"`
}

func hostGameReporting(c *Conn, h *hostSession, p blaze.Packet) []byte {
	switch p.Command {
	case 0x0064, 0x0065: 
		final := p.Command == 0x0065
		logger.Info("HOST %d: game report (final=%t, %d bytes) -> accepted", c.ID, final, len(p.Payload))
		logger.HexDump(logger.LevelDebug, "HOST GAME REPORT", p.Payload)
		return append(emptyReply(p), hostNotify(GameReporting, 0x0072, &gameReportResultNotify{FNL: final, GHID: 1000000, GRID: 1000000})...)
	default:
		logger.Warn("HOST %d: unknown GameReporting command %d (0x%04X)", c.ID, p.Command, p.Command)
		return emptyReply(p)
	}
}

func hostClosed(c *Conn, h *hostSession) {
	hs := h.snapshot()
	for _, gid := range server.Games.GamesOfHost(hs.PersonaID) {
		inGame := false
		if _, ok := server.Games.FindPlayer(gid, currentPersonaID()); ok {
			inGame = true
		}
		server.Games.Destroy(gid)
		logger.Info("HOST %d: %q disconnected -> game %d removed", c.ID, hs.Name, gid)

		if inGame {
			if push := currentPush(); push != nil {
				_ = push(gmNotify(notifyGameRemoved, &server.NotifyGameRemoved{GID: gid, REAS: 0}))
			}
		}
	}
}
