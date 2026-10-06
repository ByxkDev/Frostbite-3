package components

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bf4/blaze"
	"bf4/logger"
	"bf4/server"
)

const (
	GameManager uint16 = 0x0004

	notifyGameListUpdate uint16 = 0x00C9

	notifyMatchmakingFailed uint16 = 0x000A 
	notifyGameRemoved       uint16 = 0x0010 
	notifyGameSetup         uint16 = 0x0014 
	notifyPlayerJoining     uint16 = 0x0015 
	notifyPlayerRemoved     uint16 = 0x0028 
	notifyGameStateChange   uint16 = 0x0064 
	gameListMaxFitScore = 0x5460
)

var nextGameListID uint32

var (
	gmTdfEncoder = component35TdfFactory.CreateEncoder(true)
	gmTdfDecoder = component35TdfFactory.CreateDecoder(true)
)

func gmReply(p blaze.Packet, obj interface{}, name string) []byte {
	payload, err := gmTdfEncoder.Encode(obj)
	if err != nil {
		logger.Error("BLAZE: %s encode failed: %v", name, err)
		return nil
	}
	out := encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), payload)
	logResponse(name, out)
	return out
}

type getGameListRequest struct {
	DNAM string `tdf:"DNAM"` 
	GVER string `tdf:"GVER"`
	LCAP uint32 `tdf:"LCAP"` 
}

type getGameListResponse struct {
	GLID uint32 `tdf:"GLID"`
	MAXF uint32 `tdf:"MAXF"` 
	NGD  uint32 `tdf:"NGD"`  
}

type gameListUpdate struct {
	DONE bool                          `tdf:"DONE"` 
	GLID uint32                        `tdf:"GLID"`
	REMV []uint64                      `tdf:"REMV"`
	UPDT []server.GameBrowserMatchData `tdf:"UPDT"` 
}

type destroyGameListRequest struct {
	GLID uint32 `tdf:"GLID"`
}

type gameBrowserDataList struct {
	GDAT []server.GameBrowserGameData `tdf:"GDAT"`
}

var gamesOnce sync.Once

var ListedGames = true

func InitGames() {
	gamesOnce.Do(func() {
		if !ListedGames {
			logger.Info("GAMEMANAGER: listed games off, only hosted (dedicated) games are shown")
			return
		}
		if err := server.LoadGames(server.Games, ServerHost); err != nil {
			logger.Error("GAMEMANAGER: %v", err)
		}
	})
}

func browserGames() []server.GameBrowserMatchData {
	var out []server.GameBrowserMatchData
	for _, g := range server.Games.List() {
		if g.Settings&server.SettingOpenToBrowsing == 0 {
			continue
		}
		out = append(out, server.GameBrowserMatchData{FIT: gameListMaxFitScore, GAM: server.Games.BrowserData(g)})
	}
	if out == nil {
		out = []server.GameBrowserMatchData{}
	}
	return out
}

func HandleGameManager(p blaze.Packet) []byte {
	InitGames()

	switch p.Command {
	case 0x0001:
		return handleCreateGame(p)
	case 0x0002:
		return handleDestroyGame(p)
	case 0x0003:
		return handleAdvanceGameState(p)
	case 0x0007:
		return handleSetGameAttributes(p)
	case 0x0009:
		return handleJoinGame(p)
	case 0x000B:
		return handleRemovePlayer(p)
	case 0x001D: 
		return handleClientUpdateMeshConnection(p)
	case 0x000D: 
		return handleStartMatchmaking(p)
	case 0x000E:
		return handleCancelMatchmaking(p)
	case 0x000F:
		return handleClientFinalizeGameCreation(p)
	case 0x0064, 0x0065: 
		return handleGetGameList(p)
	case 0x0066:
		var req destroyGameListRequest
		_ = gmTdfDecoder.Decode(p.Payload, &req)
		logger.Info("GAMEMANAGER: destroyGameList GLID=%d", req.GLID)
		return emptyReply(p)
	case 0x0067: 
		return handleGetFullGameData(p)
	case 0x0069: 
		return handleGetGameDataFromID(p)
	case 0x0071: 
		logger.Info("GAMEMANAGER: command 0x71 (game data by user) -> empty GDAT")
		return gmReply(p, &gameBrowserDataList{GDAT: []server.GameBrowserGameData{}}, "GameManager 0x71")
	default:
		logger.Warn("GAMEMANAGER: unknown command %d (0x%04X)", p.Command, p.Command)
		logger.Hex(logger.LevelWarn, "GAMEMANAGER RAW", p.Payload)
		return nil
	}
}

func emptyReply(p blaze.Packet) []byte {
	return encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), nil)
}

func gmNotify(cmd uint16, obj interface{}) []byte {
	payload, err := gmTdfEncoder.Encode(obj)
	if err != nil {
		logger.Error("GAMEMANAGER: encode notification 0x%04X failed: %v", cmd, err)
		return nil
	}
	return encodeBlazePacket(GameManager, cmd, 0, blazeTypeNotification, 0, payload)
}

func handleGetGameList(p blaze.Packet) []byte {
	var req getGameListRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("GAMEMANAGER: getGameList decode failed: %v", err)
	}

	listID := atomic.AddUint32(&nextGameListID, 1)
	games := browserGames()

	logger.Info("GAMEMANAGER: getGameList cmd=0x%02X DNAM=%q GVER=%q LCAP=%d -> GLID=%d, %d games",
		p.Command, req.DNAM, req.GVER, req.LCAP, listID, len(games))

	replyPayload, err := gmTdfEncoder.Encode(&getGameListResponse{
		GLID: listID,
		MAXF: gameListMaxFitScore,
		NGD:  uint32(len(games)),
	})
	if err != nil {
		logger.Error("GAMEMANAGER: encode reply failed: %v", err)
		return nil
	}

	notifyPayload, err := gmTdfEncoder.Encode(&gameListUpdate{
		DONE: true,
		GLID: listID,
		REMV: []uint64{},
		UPDT: games,
	})
	if err != nil {
		logger.Error("GAMEMANAGER: encode NotifyGameListUpdate failed: %v", err)
		return nil
	}

	reply := encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), replyPayload)
	notify := encodeBlazePacket(GameManager, notifyGameListUpdate, 0, blazeTypeNotification, 0, notifyPayload)
	logger.HexDump(logger.LevelDebug, "GAMEMANAGER getGameList REPLY", reply)
	logger.HexDump(logger.LevelDebug, "GAMEMANAGER NotifyGameListUpdate", notify)
	return append(reply, notify...)
}

var PushNotification func(b []byte) error

var (
	pushMu    sync.Mutex
	pushToken uint64
)

func SetPushNotification(fn func(b []byte) error) uint64 {
	pushMu.Lock()
	defer pushMu.Unlock()
	pushToken++
	PushNotification = fn
	return pushToken
}

func ClearPushNotification(token uint64) {
	pushMu.Lock()
	defer pushMu.Unlock()
	if token == pushToken {
		PushNotification = nil
	}
}

func currentPush() func(b []byte) error {
	pushMu.Lock()
	defer pushMu.Unlock()
	return PushNotification
}

func CurrentPersonaID() uint64 { return currentPersonaID() }

const (
	mmResultTimedOut = 3 
	mmResultCanceled = 4 
)

type startMatchmakingRequest struct {
	ATTR map[string]string `tdf:"ATTR"` 
	DUR  uint32            `tdf:"DUR"` 
	GSET uint32            `tdf:"GSET"` 
	MODE uint32            `tdf:"MODE"`
	PMAX uint16            `tdf:"PMAX"` 
}

type startMatchmakingResponse struct {
	MSID uint32 `tdf:"MSID"`
}

type cancelMatchmakingRequest struct {
	MSID uint32 `tdf:"MSID"`
}

type matchmakingFailedNotify struct {
	MAXF uint32 `tdf:"MAXF"`
	MSID uint32 `tdf:"MSID"`
	RSLT uint32 `tdf:"RSLT"`
	USID uint64 `tdf:"USID"`
}

var (
	nextMatchmakingID uint32
	mmMu              sync.Mutex
	mmTimers          = map[uint32]*time.Timer{}
)

func matchmakingFailed(msid uint32, result uint32, usid uint64) []byte {
	payload, err := gmTdfEncoder.Encode(&matchmakingFailedNotify{
		MAXF: gameListMaxFitScore,
		MSID: msid,
		RSLT: result,
		USID: usid,
	})
	if err != nil {
		logger.Error("GAMEMANAGER: encode NotifyMatchmakingFailed failed: %v", err)
		return nil
	}
	return encodeBlazePacket(GameManager, notifyMatchmakingFailed, 0, blazeTypeNotification, 0, payload)
}

func handleStartMatchmaking(p blaze.Packet) []byte {
	var req startMatchmakingRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: startMatchmaking decode stopped early (RRST map): %v", err)
	}

	msid := atomic.AddUint32(&nextMatchmakingID, 1)
	dur := time.Duration(req.DUR) * time.Millisecond
	if dur < time.Second || dur > 2*time.Minute {
		dur = 15 * time.Second
	}

	reply := gmReply(p, &startMatchmakingResponse{MSID: msid}, "GameManager startMatchmaking")
	if reply == nil {
		return nil
	}

	if PeerHostedMatchmaking {
		return handlePeerMatchmaking(p, msid, reply)
	}

	level, mode := req.ATTR["level"], req.ATTR["mode"]
	if g, ok := server.Games.FindMatch(level, mode); ok {
		g, player, err := server.Games.Join(g.ID, currentPlayerInfo())
		if err == nil {
			logger.Info("GAMEMANAGER: startMatchmaking MSID=%d level=%q mode=%q -> joined game %d %q (team %d)",
				msid, level, mode, g.ID, g.Name, player.Team)
			reason := server.GameSetupReason{}
			_ = blaze.UnionSetValue(&reason, &server.MatchmakingSetupContext{
				FIT:  gameListMaxFitScore,
				MAXF: gameListMaxFitScore,
				MSID: msid,
				RSLT: server.MatchJoinedExistingGame,
				USID: uint32(currentPersonaID()),
			})
			return append(reply, bf4JoinSequence(p, g, *player, reason, false)...)
		}
		logger.Info("GAMEMANAGER: startMatchmaking MSID=%d: join game %d failed: %v", msid, g.ID, err)
	}

	logger.Info("GAMEMANAGER: startMatchmaking MSID=%d level=%q mode=%q PMAX=%d DUR=%s -> no game fits, will time out",
		msid, level, mode, req.PMAX, dur)

	push := currentPush()
	if push == nil {
		logger.Info("GAMEMANAGER: PushNotification not set, sending timed-out result immediately")
		return append(reply, matchmakingFailed(msid, mmResultTimedOut, currentPersonaID())...)
	}

	usid, who := currentPersonaID(), GetConsoleOnlineID()
	mmMu.Lock()
	mmTimers[msid] = time.AfterFunc(dur, func() {
		mmMu.Lock()
		_, active := mmTimers[msid]
		delete(mmTimers, msid)
		mmMu.Unlock()
		if !active {
			return
		}
		logger.Info("GAMEMANAGER: matchmaking MSID=%d for %q timed out -> NotifyMatchmakingFailed", msid, who)
		if err := push(matchmakingFailed(msid, mmResultTimedOut, usid)); err != nil {
			logger.Warn("GAMEMANAGER: push NotifyMatchmakingFailed failed: %v", err)
		}
	})
	mmMu.Unlock()

	return reply
}

func handleCancelMatchmaking(p blaze.Packet) []byte {
	var req cancelMatchmakingRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("GAMEMANAGER: cancelMatchmaking decode failed: %v", err)
	}

	mmMu.Lock()
	if t, ok := mmTimers[req.MSID]; ok {
		t.Stop()
		delete(mmTimers, req.MSID)
	}
	mmMu.Unlock()

	logger.Info("GAMEMANAGER: cancelMatchmaking MSID=%d", req.MSID)
	reply := encodeBlazePacket(p.Component, p.Command, 0, blazeTypeReply, uint32(p.MessageId), nil)
	return append(reply, matchmakingFailed(req.MSID, mmResultCanceled, currentPersonaID())...)
}

func currentPlayerInfo() server.PlayerInfo {
	info := server.PlayerInfo{
		PersonaID:  currentPersonaID(),
		Name:       GetConsoleOnlineID(),
		ExternalID: GetConsoleExti(),
	}
	userStateMu.Lock()
	if lastAddress != nil {
		info.Address = server.IpPairAddress{
			EXIP: server.IpAddress{IP: lastAddress.EXIP.IP, PORT: lastAddress.EXIP.PORT},
			INIP: server.IpAddress{IP: lastAddress.INIP.IP, PORT: lastAddress.INIP.PORT},
		}
	}
	userStateMu.Unlock()
	return info
}

func gameSetupNotify(g *server.Game, reason server.GameSetupReason) []byte {
	return gmNotify(notifyGameSetup, &server.NotifyGameSetup{
		GAME: server.Games.ReplicatedData(g),
		PROS: server.Games.Roster(g),
		REAS: reason,
	})
}

func datalessReason(ctx uint32) server.GameSetupReason {
	reason := server.GameSetupReason{}
	_ = blaze.UnionSetValue(&reason, &server.DatalessSetupContext{DCTX: ctx})
	return reason
}

func handleJoinGame(p blaze.Packet) []byte {
	var req server.JoinGameRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: joinGame decode stopped early: %v", err)
	}

	g, player, err := server.Games.Join(req.GID, currentPlayerInfo())
	if err != nil {
		code := gmErrGameNotFound
		if err == server.ErrGameFull {
			code = gmErrGameFull
		}
		logger.Warn("GAMEMANAGER: joinGame GID=%d failed: %v", req.GID, err)
		return encodeBlazePacket(p.Component, p.Command, code, 0x3000, uint32(p.MessageId), nil)
	}

	if req.GVER != "" && g.Version != "" && !strings.EqualFold(req.GVER, g.Version) {
		logger.Warn("GAMEMANAGER: joinGame GID=%d: client GVER %q != game VSTR %q", g.ID, req.GVER, g.Version)
	}

	logger.Info("GAMEMANAGER: joinGame GID=%d %q -> %q joined team %d slot %d (%d/%d)",
		g.ID, g.Name, player.Name, player.Team, player.Slot, server.Games.PlayerCount(g), g.MaxPlayers)

	reply := gmReply(p, &server.JoinGameResponse{GID: g.ID, JGS: 0}, "GameManager joinGame")
	return append(reply, bf4JoinSequence(p, g, *player, datalessReason(setupJoinByBrowsing), true)...)
}

func handleRemovePlayer(p blaze.Packet) []byte {
	var req server.RemovePlayerRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: removePlayer decode stopped early: %v", err)
	}
	if req.PID == 0 {
		req.PID = currentPersonaID()
	}
	left := server.Games.Leave(req.GID, req.PID)
	logger.Info("GAMEMANAGER: removePlayer GID=%d PID=%d REAS=%d (was in game: %t)", req.GID, req.PID, req.REAS, left)

	if left {
		notifyPlayersOfRemoval(req.GID, req.PID)
	}
	return append(emptyReply(p), gmNotify(notifyPlayerRemoved, &server.NotifyPlayerRemoved{
		GID: req.GID, PID: req.PID, REAS: req.REAS,
	})...)
}

func handleGetGameDataFromID(p blaze.Packet) []byte {
	var req server.GetGameDataFromIDRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: getGameDataFromId decode stopped early: %v", err)
	}
	resp := gameBrowserDataList{GDAT: []server.GameBrowserGameData{}}
	for _, id := range req.GIDS {
		if g, ok := server.Games.Get(id); ok {
			resp.GDAT = append(resp.GDAT, server.Games.BrowserData(g))
		}
	}
	logger.Info("GAMEMANAGER: getGameDataFromId %v -> %d games", req.GIDS, len(resp.GDAT))
	return gmReply(p, &resp, "GameManager getGameDataFromId")
}

const (
	gmErrGameNotFound uint16 = 0x0002
	gmErrGameFull     uint16 = 0x0004
)

func handleCreateGame(p blaze.Packet) []byte {
	var req server.CreateGameRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: createGame decode stopped early: %v", err)
	}

	fallbackIP, fallbackPort := uint32(0), uint16(server.DefaultGamePort)
	userStateMu.Lock()
	if lastAddress != nil {
		fallbackIP = lastAddress.INIP.IP
	}
	userStateMu.Unlock()

	g := server.Games.CreateFromRequest(req, currentPersonaID(), fallbackIP, fallbackPort)
	logger.Info("GAMEMANAGER: createGame -> GID=%d %q level=%q mode=%q max=%d host=%d",
		g.ID, g.Name, g.Level, g.Mode, g.MaxPlayers, g.HostID)

	reply := gmReply(p, &server.CreateGameResponse{GID: g.ID}, "GameManager createGame")
	return append(reply, gameSetupNotify(g, datalessReason(server.SetupCreateGame))...)
}

func handleAdvanceGameState(p blaze.Packet) []byte {
	var req server.AdvanceGameStateRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: advanceGameState decode stopped early: %v", err)
	}
	ok := server.Games.SetState(req.GID, req.GSTA)
	logger.Info("GAMEMANAGER: advanceGameState GID=%d -> %d (%s) (found=%t)", req.GID, req.GSTA, gameStateName(req.GSTA), ok)
	changed := gmNotify(notifyGameStateChange, &server.NotifyGameStateChange{GID: req.GID, GSTA: req.GSTA})
	for _, pid := range server.Games.PlayerIDs(req.GID) {
		if pid != currentPersonaID() {
			pushToPersona(pid, changed)
		}
	}
	return append(emptyReply(p), changed...)
}

func handleSetGameAttributes(p blaze.Packet) []byte {
	var req server.SetGameAttributesRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: setGameAttributes decode stopped early: %v", err)
	}
	ok := server.Games.SetAttributes(req.GID, req.ATTR)
	logger.Info("GAMEMANAGER: setGameAttributes GID=%d %v (found=%t)", req.GID, req.ATTR, ok)
	return emptyReply(p)
}

func handleDestroyGame(p blaze.Packet) []byte {
	var req server.GameIDRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: destroyGame decode stopped early: %v", err)
	}
	players := server.Games.PlayerIDs(req.GID)
	ok := server.Games.Destroy(req.GID)
	logger.Info("GAMEMANAGER: destroyGame GID=%d (removed=%t)", req.GID, ok)
	if ok {
		removed := gmNotify(notifyGameRemoved, &server.NotifyGameRemoved{GID: req.GID})
		for _, pid := range players {
			if pid != currentPersonaID() {
				pushToPersona(pid, removed)
			}
		}
	}
	reply := gmReply(p, &server.CreateGameResponse{GID: req.GID}, "GameManager destroyGame")
	return append(reply, gmNotify(notifyGameRemoved, &server.NotifyGameRemoved{GID: req.GID})...)
}

func handleGetFullGameData(p blaze.Packet) []byte {
	var req server.GetFullGameDataRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: getFullGameData decode stopped early: %v", err)
	}

	resp := server.GetFullGameDataResponse{LGAM: []server.ListGameData{}}
	for _, id := range req.GIDL {
		g, ok := server.Games.Get(id)
		if !ok {
			logger.Warn("GAMEMANAGER: getFullGameData: no game %d", id)
			continue
		}
		resp.LGAM = append(resp.LGAM, server.ListGameData{
			GAME: server.Games.ReplicatedData(g),
			PROS: server.Games.Roster(g),
		})
	}

	logger.Info("GAMEMANAGER: getFullGameData %v -> %d games", req.GIDL, len(resp.LGAM))
	return gmReply(p, &resp, "GameManager getFullGameData")
}

const setupJoinByBrowsing uint32 = 3

func bf4JoinSequence(p blaze.Packet, g *server.Game, player server.Player, reason server.GameSetupReason, platformHostAsReply bool) []byte {
	hostID, hostName := g.HostPersona(), g.HostDisplayName()

	var out []byte

	phi := &server.NotifyPlatformHostInitialized{GID: g.ID, PHID: hostID}
	if platformHostAsReply {
		if payload, err := gmTdfEncoder.Encode(phi); err == nil {
			out = append(out, encodeBlazePacket(GameManager, notifyPlatformHostInit, 0, blazeTypeReply, uint32(p.MessageId), payload)...)
		}
	} else {
		out = append(out, gmNotify(notifyPlatformHostInit, phi)...)
	}

	out = append(out, userAddedPacket(hostID, hostName)...)
	out = append(out, hostNotify(UserSessions, 0x0005, &userUpdatedNotify{FLGS: 3, ID: hostID})...)

	out = append(out, gmNotify(notifyJoiningPlayerInitConns, &server.NotifyJoiningPlayerInitiateConnections{
		GAME: server.Games.BF4Data(g, &player),
		LFPJ: 0,
		PROS: server.Games.BF4Roster(g),
		REAS: reason,
	})...)

	logger.Info("GAMEMANAGER: join sequence for %q -> game %d, host %q (%d) at %s:%d",
		player.Name, g.ID, hostName, hostID, ipString(g.HostIP), g.HostPort)

	notifyHostOfJoin(g, player)
	notifyPlayersOfJoin(g, player)
	return out
}

func notifyPlayersOfJoin(g *server.Game, player server.Player) {
	var b []byte
	b = append(b, userAddedPacket(player.PersonaID, player.Name)...)
	b = append(b, hostNotify(UserSessions, 0x0005, &userUpdatedNotify{FLGS: 3, ID: player.PersonaID})...)
	b = append(b, gmNotify(notifyPlayerJoining, &server.NotifyPlayerJoiningBF4{GID: g.ID, PDAT: server.BF4PlayerEntry(g.ID, player)})...)

	told := 0
	for _, pid := range server.Games.PlayerIDs(g.ID) {
		if pid == player.PersonaID || pid == g.HostID {
			continue
		}
		pushToPersona(pid, b)
		told++
	}
	if told > 0 {
		logger.Info("GAMEMANAGER: %d other player(s) in game %d told that %q joined", told, g.ID, player.Name)
	}
}

func notifyHostOfJoin(g *server.Game, player server.Player) {
	push := server.Games.HostPushFor(g.ID)
	if push == nil {
		logger.Info("GAMEMANAGER: game %d has no connected host (listed game); nobody to notify", g.ID)
		return
	}

	entry := server.BF4PlayerEntry(g.ID, player)
	var b []byte
	b = append(b, userAddedPacket(player.PersonaID, player.Name)...)
	b = append(b, hostNotify(UserSessions, 0x0005, &userUpdatedNotify{FLGS: 3, ID: player.PersonaID})...)
	b = append(b, gmNotify(notifyPlayerJoining, &server.NotifyPlayerJoiningBF4{GID: g.ID, PDAT: entry})...)
	b = append(b, gmNotify(notifyPlayerClaimingReserv, &server.NotifyPlayerJoiningBF4{GID: g.ID, PDAT: entry})...)
	b = append(b, extendedDataUpdateNotify(player.PersonaID)...)

	if err := push(b); err != nil {
		logger.Warn("GAMEMANAGER: notify host of game %d failed: %v", g.ID, err)
		return
	}
	logger.Info("GAMEMANAGER: host of game %d told that %q (%d) is joining", g.ID, player.Name, player.PersonaID)
}

func handleClientUpdateMeshConnection(p blaze.Packet) []byte {
	var req server.UpdateMeshConnectionRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: updateMeshConnection decode stopped early: %v", err)
	}
	
	if r := peerHostMeshUpdate(p, req); r != nil {
		return r
	}
	pid := currentPersonaID()
	reply := emptyReply(p)

	if req.STAT == 2 {
		server.Games.SetPlayerState(req.GID, pid, playerStateConnected)
		logger.Info("GAMEMANAGER: updateMeshConnection GID=%d: %q CONNECTED to the host", req.GID, GetConsoleOnlineID())
		connected := append(
			gmNotify(notifyGamePlayerStateChange, &server.NotifyGamePlayerStateChange{GID: req.GID, PID: pid, STAT: playerStateConnected}),
			gmNotify(notifyPlayerJoinCompleted, &server.NotifyPlayerJoinCompleted{GID: req.GID, PID: pid})...)
		for _, other := range server.Games.PlayerIDs(req.GID) {
			if other != pid {
				pushToPersona(other, connected)
			}
		}
		return append(reply, connected...)
	}

	if server.Games.Leave(req.GID, pid) {
		notifyPlayersOfRemoval(req.GID, pid)
	}
	logger.Info("GAMEMANAGER: updateMeshConnection GID=%d STAT=%d: %q could not connect to the host -> removed", req.GID, req.STAT, GetConsoleOnlineID())
	return append(reply, gmNotify(notifyPlayerRemoved, &server.NotifyPlayerRemoved{GID: req.GID, PID: pid, REAS: removeReasonLeft})...)
}

type finalizeGameCreationRequest struct {
	GID  uint32 `tdf:"GID"`
	NPSI string `tdf:"NPSI"` 
	XNNC []byte `tdf:"XNNC"`
	XSES []byte `tdf:"XSES"`
}

func handleClientFinalizeGameCreation(p blaze.Packet) []byte {
	var req finalizeGameCreationRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("GAMEMANAGER: finalizeGameCreation decode stopped early: %v", err)
	}
	reply := emptyReply(p)

	g, ok := server.Games.Get(req.GID)
	me := currentPersonaID()
	if !ok || g.HostID != me {
		logger.Info("GAMEMANAGER: finalizeGameCreation GID=%d -> empty reply", req.GID)
		return reply
	}

	logger.Info("PEER: host %q finalized game %d (NPSI=%q) -> NotifyPlatformHostInitialized", GetConsoleOnlineID(), req.GID, req.NPSI)
	return append(reply, gmNotify(notifyPlatformHostInit, &server.NotifyPlatformHostInitialized{
		GID: req.GID, PHID: me, PHST: 0,
	})...)
}

func gameStateName(s uint32) string {
	switch s {
	case server.GameStateNew:
		return "NEW"
	case server.GameStateInitializing:
		return "INITIALIZING"
	case server.GameStatePreGame:
		return "PRE_GAME"
	case server.GameStateInGame:
		return "IN_GAME"
	case server.GameStatePostGame:
		return "POST_GAME"
	case server.GameStateDestructing:
		return "DESTRUCTING"
	}
	return "?"
}