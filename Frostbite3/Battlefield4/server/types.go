package server

import "bf4/blaze"

const (
	GameStateNew          uint32 = 0
	GameStateInitializing uint32 = 1
	GameStatePostGame     uint32 = 4
	GameStateDestructing  uint32 = 6
	GameStatePreGame      uint32 = 130
	GameStateInGame       uint32 = 131
)

const (
	PlayerStateReserved         uint32 = 0
	PlayerStateActiveConnecting uint32 = 2
	PlayerStateActiveConnected  uint32 = 4
)

const (
	TopologyPeerHosted uint32 = 0
	TopologyDedicated  uint32 = 1
)

const (
	MatchJoinedExistingGame uint32 = 2
)

const (
	SetupCreateGame uint32 = 0
	SetupJoinGame   uint32 = 1
)

const (
	SettingOpenToBrowsing     uint32 = 1
	SettingOpenToMatchmaking  uint32 = 2
	SettingOpenToInvites      uint32 = 4
	SettingOpenToJoinByPlayer uint32 = 8
	SettingRanked             uint32 = 32
)

type IpAddress struct {
	IP   uint32 `tdf:"IP"`
	PORT uint16 `tdf:"PORT"`
}

type IpPairAddress struct {
	EXIP IpAddress `tdf:"EXIP"`
	INIP IpAddress `tdf:"INIP"`
}

type NetworkAddress struct {
	blaze.Union
	VALU *IpPairAddress `tdfunion:"2"`
}

func NewIpPairAddress(ip uint32, port uint16) NetworkAddress {
	pair := IpPairAddress{EXIP: IpAddress{IP: ip, PORT: port}, INIP: IpAddress{IP: ip, PORT: port}}
	addr := NetworkAddress{}
	_ = blaze.UnionSetValue(&addr, &pair)
	return addr
}

type HostInfo struct {
	HPID uint64 `tdf:"HPID"` 
	HSLT uint8  `tdf:"HSLT"` 
}

type NetworkQosData struct {
	DBPS uint32 `tdf:"DBPS"`
	NATT uint32 `tdf:"NATT"`
	UBPS uint32 `tdf:"UBPS"`
}

type ReplicatedGameData struct {
	ADMN []uint64          `tdf:"ADMN"` 
	ATTR map[string]string `tdf:"ATTR"`
	CAP  []uint16          `tdf:"CAP"`  
	GID  uint32            `tdf:"GID"`
	GNAM string            `tdf:"GNAM"`
	GPVH uint64            `tdf:"GPVH"`
	GSET uint32            `tdf:"GSET"`
	GSID uint64            `tdf:"GSID"` 
	GSTA uint32            `tdf:"GSTA"`
	GTYP string            `tdf:"GTYP"`
	HNET []NetworkAddress  `tdf:"HNET"` 
	HSES uint32            `tdf:"HSES"`
	IGNO bool              `tdf:"IGNO"`
	MCAP uint16            `tdf:"MCAP"` 
	NQOS NetworkQosData    `tdf:"NQOS"`
	NRES bool              `tdf:"NRES"`
	NTOP uint32            `tdf:"NTOP"`
	PGID string            `tdf:"PGID"`
	PHST HostInfo          `tdf:"PHST"`
	PRES uint32            `tdf:"PRES"`
	PSAS string            `tdf:"PSAS"` 
	QCAP uint16            `tdf:"QCAP"`
	SEED uint32            `tdf:"SEED"`
	TCAP uint16            `tdf:"TCAP"`
	THST HostInfo          `tdf:"THST"`
	TIDS []uint16          `tdf:"TIDS"`
	UUID string            `tdf:"UUID"`
	VOIP uint32            `tdf:"VOIP"`
	VSTR string            `tdf:"VSTR"` 
}

type ReplicatedGamePlayer struct {
	EXID uint64            `tdf:"EXID"` 
	GID  uint32            `tdf:"GID"`
	LOC  uint32            `tdf:"LOC"`
	NAME string            `tdf:"NAME"`
	PATT map[string]string `tdf:"PATT"`
	PID  uint64            `tdf:"PID"`
	PNET NetworkAddress    `tdf:"PNET"`
	SID  uint8             `tdf:"SID"`
	SLOT uint32            `tdf:"SLOT"`
	STAT uint32            `tdf:"STAT"`
	TIDX uint16            `tdf:"TIDX"`
	TIME uint64            `tdf:"TIME"`
	UID  uint32            `tdf:"UID"`
}

type GameSetupReason struct {
	blaze.Union
	Dataless    *DatalessSetupContext    `tdfunion:"0"`
	Matchmaking *MatchmakingSetupContext `tdfunion:"3"`
}

type DatalessSetupContext struct {
	DCTX uint32 `tdf:"DCTX"`
}

type MatchmakingSetupContext struct {
	FIT  uint32 `tdf:"FIT"`
	MAXF uint32 `tdf:"MAXF"`
	MSID uint32 `tdf:"MSID"`
	RSLT uint32 `tdf:"RSLT"`
	USID uint32 `tdf:"USID"`
}

type NotifyGameSetup struct {
	GAME ReplicatedGameData     `tdf:"GAME"`
	PROS []ReplicatedGamePlayer `tdf:"PROS"`
	REAS GameSetupReason        `tdf:"REAS"`
}

type GameBrowserPlayerData struct {
	EXID uint64            `tdf:"EXID"`
	LOC  uint32            `tdf:"LOC"`
	NAME string            `tdf:"NAME"`
	PATT map[string]string `tdf:"PATT"`
	PID  uint64            `tdf:"PID"`
	STAT uint32            `tdf:"STAT"`
	TIDX uint16            `tdf:"TIDX"`
}

type GameBrowserGameData struct {
	ADMN []uint64                `tdf:"ADMN"`
	ATTR map[string]string       `tdf:"ATTR"`
	CAP  []uint16                `tdf:"CAP"`
	GID  uint32                  `tdf:"GID"`
	GNAM string                  `tdf:"GNAM"`
	GSET uint32                  `tdf:"GSET"`
	GSTA uint32                  `tdf:"GSTA"`
	HNET []NetworkAddress        `tdf:"HNET"`
	HOST uint64                  `tdf:"HOST"`
	NTOP uint32                  `tdf:"NTOP"`
	PCNT []uint16                `tdf:"PCNT"` 
	PRES uint32                  `tdf:"PRES"`
	PSAS string                  `tdf:"PSAS"`
	QCAP uint16                  `tdf:"QCAP"`
	QCNT uint16                  `tdf:"QCNT"`
	ROST []GameBrowserPlayerData `tdf:"ROST"`
	TCAP uint16                  `tdf:"TCAP"`
	VOIP uint32                  `tdf:"VOIP"`
	VSTR string                  `tdf:"VSTR"`
}

type GameBrowserMatchData struct {
	FIT uint32              `tdf:"FIT"`
	GAM GameBrowserGameData `tdf:"GAM"`
}

type CreateGameRequest struct {
	ATTR map[string]string `tdf:"ATTR"`
	GNAM string            `tdf:"GNAM"`
	GSET uint32            `tdf:"GSET"`
	GTYP string            `tdf:"GTYP"`
	HNET []NetworkAddress  `tdf:"HNET"`
	NTOP uint32            `tdf:"NTOP"`
	PCAP []uint16          `tdf:"PCAP"`
	PMAX uint16            `tdf:"PMAX"`
	QCAP uint16            `tdf:"QCAP"`
	TCAP uint16            `tdf:"TCAP"`
	VSTR string            `tdf:"VSTR"`
}

type CreateGameResponse struct {
	GID uint32 `tdf:"GID"`
}

type JoinGameRequest struct {
	GID  uint32         `tdf:"GID"`
	GVER string         `tdf:"GVER"`
	PNET NetworkAddress `tdf:"PNET"`
	TIDX uint16         `tdf:"TIDX"`
}

type JoinGameResponse struct {
	GID uint32 `tdf:"GID"`
	JGS uint32 `tdf:"JGS"`
}

type GameIDRequest struct {
	GID uint32 `tdf:"GID"`
}

type AdvanceGameStateRequest struct {
	GID  uint32 `tdf:"GID"`
	GSTA uint32 `tdf:"GSTA"`
}

type SetGameAttributesRequest struct {
	ATTR map[string]string `tdf:"ATTR"`
	GID  uint32            `tdf:"GID"`
}

type RemovePlayerRequest struct {
	GID  uint32 `tdf:"GID"`
	PID  uint64 `tdf:"PID"`
	REAS uint32 `tdf:"REAS"`
}

type GetGameDataFromIDRequest struct {
	GIDS []uint32 `tdf:"GIDS"`
}

type NotifyGameStateChange struct {
	GID  uint32 `tdf:"GID"`
	GSTA uint32 `tdf:"GSTA"`
}

type NotifyPlayerJoining struct {
	GID  uint32               `tdf:"GID"`
	PDAT ReplicatedGamePlayer `tdf:"PDAT"`
}

type NotifyPlayerRemoved struct {
	CNTX uint16 `tdf:"CNTX"`
	GID  uint32 `tdf:"GID"`
	PID  uint64 `tdf:"PID"`
	REAS uint32 `tdf:"REAS"`
}

type NotifyGameRemoved struct {
	GID  uint32 `tdf:"GID"`
	REAS uint32 `tdf:"REAS"`
}

type GetFullGameDataRequest struct {
	GIDL []uint32 `tdf:"GIDL"` 
}

type ListGameData struct {
	GAME ReplicatedGameData     `tdf:"GAME"`
	PROS []ReplicatedGamePlayer `tdf:"PROS"`
}

type GetFullGameDataResponse struct {
	LGAM []ListGameData `tdf:"LGAM"`
}

type BF4HostInfo struct {
	CSID uint32 `tdf:"CSID"` 
	HPID uint64 `tdf:"HPID"` 
	HSLT uint8  `tdf:"HSLT"` 
}

type BF4GameData struct {
	ADMN []uint64          `tdf:"ADMN"`
	ATTR map[string]string `tdf:"ATTR"`
	CAP  []uint16          `tdf:"CAP"` 
	COID string            `tdf:"COID"`
	ESNM string            `tdf:"ESNM"`
	GID  uint32            `tdf:"GID"`
	GMRG uint32            `tdf:"GMRG"` 
	GNAM string            `tdf:"GNAM"`
	GPVH uint64            `tdf:"GPVH"`
	GSET uint32            `tdf:"GSET"`
	GSID uint64            `tdf:"GSID"`
	GSTA uint32            `tdf:"GSTA"`
	GTYP string            `tdf:"GTYP"` 
	GURL string            `tdf:"GURL"`
	HNET []NetworkAddress  `tdf:"HNET"` 
	HSES uint32            `tdf:"HSES"`
	IGNO bool              `tdf:"IGNO"`
	MCAP uint16            `tdf:"MCAP"`
	MNCP uint16            `tdf:"MNCP"`
	NQOS NetworkQosData    `tdf:"NQOS"`
	NRES bool              `tdf:"NRES"`
	NTOP uint32            `tdf:"NTOP"`
	PGID string            `tdf:"PGID"` 
	PGSR []byte            `tdf:"PGSR"` 
	PHST BF4HostInfo       `tdf:"PHST"`
	PRES uint32            `tdf:"PRES"`
	PSAS string            `tdf:"PSAS"`
	QCAP uint16            `tdf:"QCAP"`
	SEED uint32            `tdf:"SEED"`
	THST BF4HostInfo       `tdf:"THST"`
	UUID string            `tdf:"UUID"`
	VOIP uint32            `tdf:"VOIP"`
	VSTR string            `tdf:"VSTR"`
	XNNC []byte            `tdf:"XNNC"`
	XSES []byte            `tdf:"XSES"`
}

type BF4Player struct {
	BLOB []byte            `tdf:"BLOB"`
	CONG uint64            `tdf:"CONG"` 
	CSID uint32            `tdf:"CSID"`
	EXID uint64            `tdf:"EXID"`
	GID  uint32            `tdf:"GID"`
	JFPS uint32            `tdf:"JFPS"`
	LOC  uint32            `tdf:"LOC"`
	NAME string            `tdf:"NAME"`
	PATT map[string]string `tdf:"PATT"`
	PID  uint64            `tdf:"PID"`
	PNET NetworkAddress    `tdf:"PNET"`
	ROLE string            `tdf:"ROLE"`
	SID  uint32            `tdf:"SID"`
	SLOT uint32            `tdf:"SLOT"`
	STAT uint32            `tdf:"STAT"`
	TIDX uint16            `tdf:"TIDX"`
	TIME uint64            `tdf:"TIME"`
	UID  uint64            `tdf:"UID"`
}

type NotifyJoiningPlayerInitiateConnections struct {
	GAME BF4GameData     `tdf:"GAME"`
	LFPJ uint32          `tdf:"LFPJ"`
	PROS []BF4Player     `tdf:"PROS"`
	REAS GameSetupReason `tdf:"REAS"`
}

type NotifyGameSetupBF4 struct {
	GAME BF4GameData `tdf:"GAME"`
	PROS []BF4Player `tdf:"PROS"`
}

type NotifyPlayerJoiningBF4 struct {
	GID  uint32    `tdf:"GID"`
	PDAT BF4Player `tdf:"PDAT"`
}

type NotifyPlatformHostInitialized struct {
	GID  uint32 `tdf:"GID"`
	PHID uint64 `tdf:"PHID"`
	PHST uint32 `tdf:"PHST"`
}

type NotifyGamePlayerStateChange struct {
	GID  uint32 `tdf:"GID"`
	PID  uint64 `tdf:"PID"`
	STAT uint32 `tdf:"STAT"`
}

type NotifyPlayerJoinCompleted struct {
	GID uint32 `tdf:"GID"`
	PID uint64 `tdf:"PID"`
}

type NotifyGameAttribChange struct {
	ATTR map[string]string `tdf:"ATTR"`
	GID  uint32            `tdf:"GID"`
}

type NotifyGameSettingsChange struct {
	ATTR uint32 `tdf:"ATTR"`
	GID  uint32 `tdf:"GID"`
}

type NotifyGameModRegisterChanged struct {
	GMID uint32 `tdf:"GMID"`
	GMRG uint32 `tdf:"GMRG"`
}

type BF4CreateGameRequest struct {
	ATTR map[string]string `tdf:"ATTR"`
	GMRG uint32            `tdf:"GMRG"`
	GNAM string            `tdf:"GNAM"`
	GSET uint32            `tdf:"GSET"`
	HNET []NetworkAddress  `tdf:"HNET"`
	NTOP uint32            `tdf:"NTOP"`
	PCAP []uint16          `tdf:"PCAP"`
	PMAX uint16            `tdf:"PMAX"`
	VSTR string            `tdf:"VSTR"`
}

type UpdateMeshConnectionRequest struct {
	GID  uint32              `tdf:"GID"`
	STAT uint32              `tdf:"STAT"` 
	TCG  blaze.BlazeObjectID `tdf:"TCG"`  
}

type SetGameSettingsRequest struct {
	GID  uint32 `tdf:"GID"`
	GSET uint32 `tdf:"GSET"`
}

type SetPlayerCapacityRequest struct {
	GID  uint32   `tdf:"GID"`
	PCAP []uint16 `tdf:"PCAP"`
}

type UpdateGameNameRequest struct {
	GID  uint32 `tdf:"GID"`
	GNAM string `tdf:"GNAM"`
}

type SetGameModRegisterRequest struct {
	GID  uint32 `tdf:"GID"`
	GMRG uint32 `tdf:"GMRG"`
}

type NotifyGameSetupMM struct {
	GAME BF4GameData     `tdf:"GAME"`
	PROS []BF4Player     `tdf:"PROS"`
	REAS GameSetupReason `tdf:"REAS"`
}

const MatchCreatedGame uint32 = 0
