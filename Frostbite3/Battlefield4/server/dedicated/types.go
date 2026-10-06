package dedicated

import "bf4/blaze"

const (
	authComponent uint16 = 0x0001
	gmComponent   uint16 = 0x0004
	utilComponent uint16 = 0x0009
	usComponent   uint16 = 0x7802

	authLogin        uint16 = 0x0028
	authLoginPersona uint16 = 0x006E

	utilPing     uint16 = 0x0002
	utilPreAuth  uint16 = 0x0007
	utilPostAuth uint16 = 0x0008

	usUpdateNetworkInfo uint16 = 0x0014

	gmCreateGame           uint16 = 0x0001
	gmDestroyGame          uint16 = 0x0002
	gmAdvanceGameState     uint16 = 0x0003
	gmFinalizeGameCreation uint16 = 0x000F
	gmUpdateMeshConnection uint16 = 0x001D

	gmNotifyGameRemoved               uint16 = 0x0010
	gmNotifyPlayerJoining             uint16 = 0x0015
	gmNotifyPlayerClaimingReservation uint16 = 0x0019
	gmNotifyPlayerRemoved             uint16 = 0x0028
)

const (
	gameStatePreGame uint32 = 130
	gameStateInGame  uint32 = 131
)

const topologyDedicatedServer uint32 = 1

const (
	meshDisconnected uint32 = 0
	meshConnected    uint32 = 2
)

const (
	settingOpenToBrowsing     uint32 = 0x0001
	settingOpenToMatchmaking  uint32 = 0x0002
	settingOpenToInvites      uint32 = 0x0004
	settingOpenToJoinByPlayer uint32 = 0x0008
	settingRanked             uint32 = 0x0020
	settingJoinInProgress     uint32 = 0x0500
)

type ipAddress struct {
	IP   uint32 `tdf:"IP"`
	PORT uint16 `tdf:"PORT"`
}

type ipPairAddress struct {
	EXIP ipAddress `tdf:"EXIP"`
	INIP ipAddress `tdf:"INIP"`
}

type networkAddress struct {
	blaze.Union
	VALU *ipPairAddress `tdfunion:"2"`
}

func newNetworkAddress(pair ipPairAddress) networkAddress {
	a := networkAddress{}
	_ = blaze.UnionSetValue(&a, &pair)
	return a
}

type clientInfo struct {
	BSDK string `tdf:"BSDK"`
	BTIM string `tdf:"BTIM"`
	CLNT string `tdf:"CLNT"` 
	CPFT uint32 `tdf:"CPFT"` 
	CSKU string `tdf:"CSKU"`
	CVER string `tdf:"CVER"`
	DSDK string `tdf:"DSDK"`
	ENV  string `tdf:"ENV"`
}

type fetchConfig struct {
	CFID string `tdf:"CFID"`
}

type preAuthRequest struct {
	CINF clientInfo  `tdf:"CINF"`
	FCCR fetchConfig `tdf:"FCCR"`
}

type loginRequest struct {
	MAIL string `tdf:"MAIL"`
	PASS string `tdf:"PASS"`
}

type loginPersonaRequest struct {
	PNAM string `tdf:"PNAM"`
}

type updateNetworkInfoRequest struct {
	ADDR networkAddress `tdf:"ADDR"`
}

type createGameRequest struct {
	ATTR map[string]string `tdf:"ATTR"` 
	GNAM string            `tdf:"GNAM"`
	GSET uint32            `tdf:"GSET"`
	GTYP string            `tdf:"GTYP"`
	HNET []networkAddress  `tdf:"HNET"`
	IGNO bool              `tdf:"IGNO"`
	NRES bool              `tdf:"NRES"`
	NTOP uint32            `tdf:"NTOP"`
	PCAP []uint16          `tdf:"PCAP"` 
	PMAX uint16            `tdf:"PMAX"`
	QCAP uint16            `tdf:"QCAP"`
	RCAP map[string]uint16 `tdf:"RCAP"` 
	VOIP uint32            `tdf:"VOIP"`
	VSTR string            `tdf:"VSTR"`
}

type createGameResponse struct {
	GID uint32 `tdf:"GID"`
}

type gameIDRequest struct {
	GID uint32 `tdf:"GID"`
}

type advanceGameStateRequest struct {
	GID  uint32 `tdf:"GID"`
	GSTA uint32 `tdf:"GSTA"`
}

type destroyGameRequest struct {
	GID  uint32 `tdf:"GID"`
	REAS uint32 `tdf:"REAS"`
}

type updateMeshConnectionRequest struct {
	GID  uint32              `tdf:"GID"`
	STAT uint32              `tdf:"STAT"`
	TCG  blaze.BlazeObjectID `tdf:"TCG"` 
}

type joiningPlayer struct {
	NAME string         `tdf:"NAME"`
	PID  uint64         `tdf:"PID"`
	PNET networkAddress `tdf:"PNET"`
	ROLE string         `tdf:"ROLE"`
	TIDX uint16         `tdf:"TIDX"`
}

type playerJoiningNotify struct {
	GID  uint32        `tdf:"GID"`
	PDAT joiningPlayer `tdf:"PDAT"`
}

type playerRemovedNotify struct {
	GID  uint32 `tdf:"GID"`
	PID  uint64 `tdf:"PID"`
	REAS uint32 `tdf:"REAS"`
}
