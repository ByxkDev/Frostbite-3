package components

import (
	"fmt"

	"bf4/blaze"
	"bf4/logger"
	"bf4/server"
)

const notifyGameCapacityChange uint16 = 0x006F

func pushToPlayer(personaID uint64, b []byte) bool {
	if personaID == 0 || len(b) == 0 {
		return false
	}
	if s := SessionByPersona(personaID); s != nil && s.Push != nil {
		pushToPersona(personaID, b)
		return true
	}
	if personaID == currentPersonaID() {
		if push := currentPush(); push != nil {
			if err := push(b); err != nil {
				logger.Warn("HOST: push to player %d failed: %v", personaID, err)
				return false
			}
			return true
		}
	}
	return false
}

func pushToGamePlayers(gid uint32, b []byte, except ...uint64) int {
	told := 0
next:
	for _, pid := range server.Games.PlayerIDs(gid) {
		for _, x := range except {
			if pid == x {
				continue next
			}
		}
		if pushToPlayer(pid, b) {
			told++
		}
	}
	return told
}

func gameObjectID(gid uint32) blaze.BlazeObjectID {
	return blaze.NewBlazeObjectID(int64(gid), blaze.NewBlazeObjectType(GameManager, 1))
}

func extendedDataUpdateInGame(personaID uint64, addr server.IpPairAddress, gid uint32) []byte {
	na := networkAddressUnion{Union: blaze.Union{ActiveMember: blaze.UnionUnsetMember}}
	if addr.EXIP.IP != 0 || addr.INIP.IP != 0 {
		pair := ipPairAddress{
			EXIP: ipAddress{IP: addr.EXIP.IP, PORT: addr.EXIP.PORT},
			INIP: ipAddress{IP: addr.INIP.IP, PORT: addr.INIP.PORT},
		}
		if pair.INIP.IP == 0 {
			pair.INIP = pair.EXIP
		}
		if err := blaze.UnionSetValue(&na, &pair); err != nil {
			logger.Error("USERSESSIONS: set ADDR union failed: %v", err)
		}
	}

	var extra []blaze.BlazeObjectID
	if gid != 0 {
		extra = append(extra, gameObjectID(gid))
	}
	data, err := encodeUserExtendedData(na, personaID, extra...)
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

func (h *hostSession) forgetGame(gid uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, id := range h.Games {
		if id == gid {
			h.Games = append(h.Games[:i], h.Games[i+1:]...)
			return
		}
	}
}

func hostAdvanceGameState(c *Conn, h *hostSession, p blaze.Packet) []byte {
	var req server.AdvanceGameStateRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("HOST %d: advanceGameState decode stopped early: %v", c.ID, err)
	}
	gid := firstGame(h, req.GID)
	ok := server.Games.SetState(gid, req.GSTA)
	logger.Info("HOST %d: advanceGameState GID=%d -> %d (%s) (found=%t)", c.ID, gid, req.GSTA, gameStateName(req.GSTA), ok)

	changed := gmNotify(notifyGameStateChange, &server.NotifyGameStateChange{GID: gid, GSTA: req.GSTA})
	if n := pushToGamePlayers(gid, changed); n > 0 {
		logger.Info("HOST %d: %d player(s) in game %d told about state %s", c.ID, n, gid, gameStateName(req.GSTA))
	}
	return append(emptyReply(p), changed...)
}

func hostDestroyGame(c *Conn, h *hostSession, p blaze.Packet) []byte {
	var req server.DestroyGameRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("HOST %d: destroyGame decode stopped early: %v", c.ID, err)
	}
	gid := firstGame(h, req.GID)
	players := server.Games.PlayerIDs(gid)
	ok := server.Games.Destroy(gid)
	h.forgetGame(gid)
	logger.Info("HOST %d: destroyGame GID=%d REAS=%d (removed=%t, %d players)", c.ID, gid, req.REAS, ok, len(players))

	removed := gmNotify(notifyGameRemoved, &server.NotifyGameRemoved{GID: gid, REAS: req.REAS})
	for _, pid := range players {
		pushToPlayer(pid, removed)
	}
	reply := gmReply(p, &server.CreateGameResponse{GID: gid}, "Host destroyGame")
	return append(reply, removed...)
}

func hostSetPlayerCapacity(c *Conn, h *hostSession, p blaze.Packet) []byte {
	var req server.SetPlayerCapacityRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("HOST %d: setPlayerCapacity decode stopped early: %v", c.ID, err)
	}
	gid := firstGame(h, req.GID)
	server.Games.SetCapacityList(gid, req.PCAP)
	capacity := server.Games.CapacityOf(gid)
	logger.Info("HOST %d: setPlayerCapacity GID=%d PCAP=%v -> CAP=%v", c.ID, gid, req.PCAP, capacity)

	changed := gmNotify(notifyGameCapacityChange, &server.NotifyGameCapacityChange{CAP: capacity, GID: gid})
	pushToGamePlayers(gid, changed)
	return append(emptyReply(p), changed...)
}

func hostRemovePlayer(c *Conn, h *hostSession, p blaze.Packet) []byte {
	var req server.RemovePlayerRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("HOST %d: removePlayer decode stopped early: %v", c.ID, err)
	}
	gid := firstGame(h, req.GID)
	others := server.Games.PlayerIDs(gid)
	left := server.Games.Leave(gid, req.PID)
	logger.Info("HOST %d: removePlayer GID=%d PID=%d REAS=%d (%s) (was in game: %t)",
		c.ID, gid, req.PID, req.REAS, removeReasonName(req.REAS), left)

	removed := gmNotify(notifyPlayerRemoved, &server.NotifyPlayerRemoved{GID: gid, PID: req.PID, REAS: req.REAS})
	if left {
		for _, pid := range others {
			pushToPlayer(pid, removed)
		}
	}
	return append(emptyReply(p), removed...)
}

func hostUpdateMeshConnection(c *Conn, h *hostSession, p blaze.Packet) []byte {
	var req server.UpdateMeshConnectionRequest
	if err := gmTdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Debug("HOST %d: updateMeshConnection decode stopped early: %v", c.ID, err)
	}

	gid := firstGame(h, req.GID)
	pid := uint64(req.TCG.ID)
	reply := emptyReply(p)

	player, inGame := server.Games.FindPlayer(gid, pid)

	logger.Info("HOST %d: updateMeshConnection GID=%d PID=%d STAT=%d inGame=%t", c.ID, gid, pid, req.STAT, inGame)

	if !inGame {
		logger.Warn("HOST %d: updateMeshConnection GID=%d PID=%d player not found", c.ID, gid, pid)
		return reply
	}

	if req.STAT == server.MeshConnected {
		server.Games.SetPlayerState(gid, pid, playerStateConnected)

		logger.Info("HOST %d: updateMeshConnection GID=%d player %d (%q) CONNECTED to the server", c.ID, gid, pid, player.Name)

		connected := append(
			gmNotify(notifyGamePlayerStateChange, &server.NotifyGamePlayerStateChange{
				GID:  gid,
				PID:  pid,
				STAT: playerStateConnected,
			}),
			gmNotify(notifyPlayerJoinCompleted, &server.NotifyPlayerJoinCompleted{
				GID: gid,
				PID: pid,
			})...,
		)

		pushToGamePlayers(gid, connected)

		out := reply
		if data := extendedDataUpdateInGame(pid, player.Address, gid); len(data) > 0 {
			out = append(out, data...)
		}

		return append(out, connected...)
	}

	logger.Warn("HOST %d: updateMeshConnection GID=%d PID=%d player=%d (%q) STAT=%d -> KEEPING PLAYER", c.ID, gid, pid, pid, player.Name, req.STAT)

	server.Games.SetPlayerState(gid, pid, playerStateConnected)

	connected := append(
		gmNotify(notifyGamePlayerStateChange, &server.NotifyGamePlayerStateChange{
			GID:  gid,
			PID:  pid,
			STAT: playerStateConnected,
		}),
		gmNotify(notifyPlayerJoinCompleted, &server.NotifyPlayerJoinCompleted{
			GID: gid,
			PID: pid,
		})...,
	)

	pushToGamePlayers(gid, connected)

	out := reply
	if data := extendedDataUpdateInGame(pid, player.Address, gid); len(data) > 0 {
		out = append(out, data...)
	}

	return append(out, connected...)
}

func hostClosed(c *Conn, h *hostSession) {
	hs := h.snapshot()
	for _, gid := range server.Games.GamesOfHost(hs.PersonaID) {
		players := server.Games.PlayerIDs(gid)
		server.Games.Destroy(gid)
		removed := gmNotify(notifyGameRemoved, &server.NotifyGameRemoved{GID: gid, REAS: 0})
		told := 0
		for _, pid := range players {
			if pushToPlayer(pid, removed) {
				told++
			}
		}
		logger.Info("HOST %d: %q disconnected -> game %d removed (%d of %d players told)", c.ID, hs.Name, gid, told, len(players))
	}
}

func hostGetTickerServer(c *Conn, h *hostSession, p blaze.Packet) []byte {
	hs := h.snapshot()
	resp := tickerServer{
		ADRS: ServerHost,
		PORT: tickerPort,
		SKEY: fmt.Sprintf("%d,%s:%d,battlefield-4-ps3,10,50,50,50,50,0,0", hs.PersonaID, ServerHost, tickerPort),
	}
	logger.Info("HOST %d: GetTickerServer -> %s:%d", c.ID, ServerHost, tickerPort)
	return gmReply(p, &resp, "Host GetTickerServer")
}

func hostSubmitGameReport(c *Conn, h *hostSession, p blaze.Packet) []byte {
	name := blazeCommandName(GameReporting, p.Command)
	logger.Info("HOST %d: %s (%d bytes) -> accepted", c.ID, name, len(p.Payload))
	logger.HexDump(logger.LevelDebug, "HOST "+name, p.Payload)
	if p.Command == 0x0001 {
		return append(emptyReply(p), hostNotify(GameReporting, 0x0072, &gameReportResultNotify{FNL: true, GHID: 1000000, GRID: 1000000})...)
	}
	return emptyReply(p)
}

func removeReasonName(r uint32) string {
	switch r {
	case server.RemoveReasonJoinTimeout:
		return "PLAYER_JOIN_TIMEOUT"
	case server.RemoveReasonConnLost:
		return "PLAYER_CONN_LOST"
	case server.RemoveReasonBlazeServerConnLost:
		return "BLAZESERVER_CONN_LOST"
	case server.RemoveReasonMigrationFailed:
		return "MIGRATION_FAILED"
	case server.RemoveReasonGameDestroyed:
		return "GAME_DESTROYED"
	case server.RemoveReasonGameEnded:
		return "GAME_ENDED"
	case server.RemoveReasonPlayerLeft:
		return "PLAYER_LEFT"
	case server.RemoveReasonGroupLeft:
		return "GROUP_LEFT"
	case server.RemoveReasonKicked:
		return "PLAYER_KICKED"
	case server.RemoveReasonKickedWithBan:
		return "PLAYER_KICKED_WITH_BAN"
	case server.RemoveReasonJoinFromQueueFail:
		return "PLAYER_JOIN_FROM_QUEUE_FAILED"
	case server.RemoveReasonReservationTimeout:
		return "PLAYER_RESERVATION_TIMEOUT"
	case server.RemoveReasonHostEjected:
		return "HOST_EJECTED"
	}
	return "?"
}

func blazeCommandName(component, command uint16) string {
	return blaze.CommandName(component, command)
}
