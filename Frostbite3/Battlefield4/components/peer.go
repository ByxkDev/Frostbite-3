package components

import (
	"fmt"

	"bf4/blaze"
	"bf4/logger"
	"bf4/server"
)

var PeerHostedMatchmaking = true
var PeerProfile = "coop"
var PeerSetupReason = "create"

const (
	gsOpenToBrowsing     uint32 = 0x001
	gsOpenToMatchmaking  uint32 = 0x002
	gsOpenToInvites      uint32 = 0x004
	gsOpenToJoinByPlayer uint32 = 0x008
	gsHostMigratable     uint32 = 0x010
	gsRanked             uint32 = 0x020
	gsJoinInProgress     uint32 = 0x100
)

const (
	voipDisabled   uint32 = 0
	voipDedicated  uint32 = 1
	voipPeerToPeer uint32 = 2
)

var (
	PeerLevel      = server.SiegeLevel
	PeerMode       = "TeamDeathMatch0"
	PeerMaxPlayers = uint16(24)
)

var (
	CoopMission    = server.SiegeLevel
	CoopDifficulty = "Normal"
	CoopGameType   = "frostbite_multiplayer" 
)

type peerProfile struct {
	label string
	opts  server.PeerGameOptions
}

func currentPeerProfile(hostName string) peerProfile {
	switch PeerProfile {
	case "coop":
		return peerProfile{
			label: "Co-op",
			opts: server.PeerGameOptions{
				Name:       fmt.Sprintf("%s's Co-op", hostName),
				Level:      CoopMission,
				Mode:       "Cooperative",
				MaxPlayers: 2,
				Settings: gsOpenToBrowsing | gsOpenToMatchmaking | gsOpenToInvites | gsOpenToJoinByPlayer | gsJoinInProgress,
				GameType: CoopGameType,
				VOIP:     voipPeerToPeer,
				Attributes: map[string]string{
					"mission":    CoopMission,
					"difficulty": CoopDifficulty,
				},
			},
		}
	default:
		return peerProfile{
			label: "Team Deathmatch",
			opts: server.PeerGameOptions{
				Name:       fmt.Sprintf("%s's Team Deathmatch", hostName),
				Level:      PeerLevel,
				Mode:       PeerMode,
				MaxPlayers: PeerMaxPlayers,
				Settings:   server.DefaultSettings,
				VOIP:       voipPeerToPeer,
			},
		}
	}
}

func handlePeerMatchmaking(p blaze.Packet, msid uint32, reply []byte) []byte {
	me := currentPlayerInfo()
	if me.Address.INIP.IP == 0 && me.Address.EXIP.IP == 0 {
		logger.Warn("PEER: %q has no network address yet (no UpdateNetworkInfo); others won't be able to reach it", me.Name)
	}
	prof := currentPeerProfile(me.Name)

	if g, ok := server.Games.FindPeerMatch(prof.opts.Level, prof.opts.Mode, me.PersonaID); ok {
		g, player, err := server.Games.Join(g.ID, me)
		if err == nil {
			logger.Info("PEER: MSID=%d %q -> joining %q's %s game %d (host %s:%d), team %d, %d/%d players",
				msid, me.Name, g.HostName, prof.label, g.ID, ipString(g.HostIP), g.HostPort, player.Team, server.Games.PlayerCount(g), g.MaxPlayers)
			reason := server.GameSetupReason{}
			_ = blaze.UnionSetValue(&reason, &server.MatchmakingSetupContext{
				FIT: gameListMaxFitScore, MAXF: gameListMaxFitScore, MSID: msid,
				RSLT: server.MatchJoinedExistingGame, USID: uint32(me.PersonaID),
			})
			return append(reply, bf4JoinSequence(p, g, *player, reason, false)...)
		}
		logger.Info("PEER: MSID=%d join game %d failed (%v), creating a new one", msid, g.ID, err)
	}

	g, host := server.Games.CreatePeerGame(me, prof.opts, currentPush())
	if host == nil {
		logger.Error("PEER: could not add host %q to new game %d", me.Name, g.ID)
		return reply
	}

	logger.Info("PEER: MSID=%d %q -> HOST of new %s game %d %q (level=%s mode=%s max=%d GSET=0x%X GTYP=%q VOIP=%d attrs=%v reason=%s) at %s:%d",
		msid, me.Name, prof.label, g.ID, g.Name, g.Level, g.Mode, g.MaxPlayers, g.Settings, prof.opts.GameType, g.VOIP,
		g.Attributes, PeerSetupReason, ipString(g.HostIP), g.HostPort)

	var reason server.GameSetupReason
	if PeerSetupReason == "create" {
		reason = datalessReason(server.SetupCreateGame)
	} else {
		_ = blaze.UnionSetValue(&reason, &server.MatchmakingSetupContext{
			FIT: gameListMaxFitScore, MAXF: gameListMaxFitScore, MSID: msid,
			RSLT: server.MatchCreatedGame, USID: uint32(me.PersonaID),
		})
	}

	setup := gmNotify(notifyGameSetup, &server.NotifyGameSetupMM{
		GAME: server.Games.BF4Data(g, nil),
		PROS: server.Games.BF4Roster(g),
		REAS: reason,
	})
	return append(reply, setup...)
}

func peerHostMeshUpdate(p blaze.Packet, req server.UpdateMeshConnectionRequest) []byte {
	me := currentPersonaID()
	target := uint64(req.TCG.ID)
	if target == 0 || target == me {
		return nil
	}
	g, ok := server.Games.Get(req.GID)
	if !ok || g.HostID != me {
		return nil
	}

	reply := emptyReply(p)
	if req.STAT == 2 {
		server.Games.SetPlayerState(g.ID, target, playerStateConnected)
		logger.Info("PEER: host %q reports player %d CONNECTED to game %d", GetConsoleOnlineID(), target, g.ID)
		connected := append(
			gmNotify(notifyGamePlayerStateChange, &server.NotifyGamePlayerStateChange{GID: g.ID, PID: target, STAT: playerStateConnected}),
			gmNotify(notifyPlayerJoinCompleted, &server.NotifyPlayerJoinCompleted{GID: g.ID, PID: target})...)
		for _, pid := range server.Games.PlayerIDs(g.ID) {
			if pid != me {
				pushToPersona(pid, connected)
			}
		}
		return append(reply, connected...)
	}

	if server.Games.Leave(g.ID, target) {
		logger.Info("PEER: host %q reports player %d DISCONNECTED (STAT=%d) from game %d", GetConsoleOnlineID(), target, req.STAT, g.ID)
		removed := gmNotify(notifyPlayerRemoved, &server.NotifyPlayerRemoved{GID: g.ID, PID: target, REAS: removeReasonLeft})
		pushToPersona(target, removed)
		for _, pid := range server.Games.PlayerIDs(g.ID) {
			if pid != me {
				pushToPersona(pid, removed)
			}
		}
		return append(reply, removed...)
	}
	return reply
}

func closePeerGamesOf(personaID uint64, name string) {
	for _, gid := range server.Games.GamesOfHost(personaID) {
		g, ok := server.Games.Get(gid)
		if !ok || !g.IsPeerHosted() {
			continue
		}
		players := server.Games.PlayerIDs(gid)
		server.Games.Destroy(gid)
		logger.Info("PEER: host %q left -> game %d removed (%d players told)", name, gid, len(players)-1)

		removed := gmNotify(notifyGameRemoved, &server.NotifyGameRemoved{GID: gid})
		for _, pid := range players {
			if pid != personaID {
				pushToPersona(pid, removed)
			}
		}
	}
}