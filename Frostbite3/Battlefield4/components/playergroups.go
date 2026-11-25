package components

import (
	"bf4/blaze"
	"bf4/logger"
)

const (
	PlayerGroups  uint16 = 0x0006
	Component0804 uint16 = 0x0804
)

type playerGroupAttrs struct {
	ATTR map[string]string `tdf:"ATTR"`
	MLIM uint64            `tdf:"MLIM"`
	NAME string            `tdf:"NAME"`
	OWNR uint64            `tdf:"OWNR"`
	PGID uint64            `tdf:"PGID"`
}

type playerGroupCreateRequest struct {
	GPID blaze.BlazeObjectID `tdf:"GPID"`
	JOIN uint64              `tdf:"JOIN"`
	PGRP playerGroupAttrs    `tdf:"PGRP"`
}

type userListRequest struct {
	HIST uint64 `tdf:"HIST"`
	IADM uint64 `tdf:"IADM"`
	LCAP uint64 `tdf:"LCAP"`
	UID  uint64 `tdf:"UID"`
}

func HandlePlayerGroups(p blaze.Packet) []byte {
	if p.Component == Component0804 {
		return HandleComponent0804(p)
	}

	switch p.Command {
	case 0x0001:
		return handlePlayerGroupsCreate(p)
	default:
		logger.Warn("PLAYERGROUPS: unknown command %d (0x%04X)", p.Command, p.Command)
		logger.Hex(logger.LevelWarn, "PLAYERGROUPS RAW", p.Payload)
		return emptyReply(p)
	}
}

func handlePlayerGroupsCreate(p blaze.Packet) []byte {
	var req playerGroupCreateRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("PLAYERGROUPS: create decode failed: %v", err)
	}
	logger.Info("PLAYERGROUPS: create GPID=%v JOIN=%d NAME=%q MLIM=%d ATTR=%v -> empty reply",
		req.GPID, req.JOIN, req.PGRP.NAME, req.PGRP.MLIM, req.PGRP.ATTR)
	return emptyReply(p)
}

func handleComponent0801Cmd20(p blaze.Packet) []byte {
	var req userListRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("0801: command 20 decode failed: %v", err)
	}
	logger.Info("0801: command 20 HIST=%d IADM=%d LCAP=%d UID=%d -> empty reply",
		req.HIST, req.IADM, req.LCAP, req.UID)
	return emptyReply(p)
}

func HandleComponent0804(p blaze.Packet) []byte {
	logger.HexDump(logger.LevelDebug, "0804 REQUEST", p.Payload)

	var req userListRequest
	if err := component35TdfDecoder.Decode(p.Payload, &req); err != nil {
		logger.Warn("0804: command %d decode failed: %v", p.Command, err)
	}
	uid := req.UID
	if uid == 0 {
		uid = currentPersonaID()
	}

	switch p.Command {
	case 0x000D:
		logger.Info("0804: command 13 HIST=%d LCAP=%d UID=%d -> empty reply", req.HIST, req.LCAP, uid)
	default:
		logger.Warn("0804: unknown command %d (0x%04X) -> empty reply", p.Command, p.Command)
		logger.Hex(logger.LevelWarn, "0804 RAW", p.Payload)
	}
	return emptyReply(p)
}
