package components

import (
	"bf4/blaze"
	"bf4/logger"
)

type rspAdminConfig struct {
	MRMX uint32 `tdf:"MRMX"`
	SAMX uint32 `tdf:"SAMX"`
	SBMX uint32 `tdf:"SBMX"`
	SVMX uint32 `tdf:"SVMX"`
	UAMX uint32 `tdf:"UAMX"`
	UOMX uint32 `tdf:"UOMX"`
}

type rspPresetSetting struct {
	KEY string `tdf:"KEY"`
	VAL uint32 `tdf:"VAL"`
}

type rspPreset struct {
	DESC string             `tdf:"DESC"`
	NAME string             `tdf:"NAME"`
	PID  uint32             `tdf:"PID"`
	PTYP string             `tdf:"PTYP"`
	SLST []rspPresetSetting `tdf:"SLST"`
	STYP uint32             `tdf:"STYP"`
}

type rspPingSite struct {
	ALIA string `tdf:"ALIA"`
	CAP  uint32 `tdf:"CAP"`
	NAME string `tdf:"NAME"`
}

type rspSettingCategory struct {
	CID  uint32 `tdf:"CID"`
	NAME string `tdf:"NAME"`
}

type rspSettingConfig struct {
	CID  uint32 `tdf:"CID"`
	DESC string `tdf:"DESC"`
	KEY  string `tdf:"KEY"`
	NAME string `tdf:"NAME"`
}

type rspSettingRange struct {
	KEY string `tdf:"KEY"`
	MAX uint32 `tdf:"MAX"`
	MIN uint32 `tdf:"MIN"`
}

type rspGetConfigResponse struct {
	ACFG rspAdminConfig       `tdf:"ACFG"`
	PCAT []rspSettingCategory `tdf:"PCAT"`
	PLST []rspPreset          `tdf:"PLST"`
	PSET []rspSettingConfig   `tdf:"PSET"`
	PSLS []rspPingSite        `tdf:"PSLS"`
	PVAL []rspSettingRange    `tdf:"PVAL"`
}

type rspSetting struct {
	key, name                    string
	normal, hard, infantry, clas uint32
	min, max                     uint32
}

var rspSettings = []rspSetting{
	{"vffi", "ID_RSP_FRIENDLY_FIRE", 0, 1, 0, 1, 0, 1},
	{"vnit", "ID_RSP_KICK_IDLE_PLAYER", 300, 300, 300, 300, 0, 86400},
	{"vaba", "ID_RSP_TEAM_BALANCE", 1, 1, 1, 1, 0, 1},
	{"vtkc", "ID_RSP_KICK_AFTER_OF_TK", 5, 5, 5, 5, 0, 99},
	{"vtkk", "ID_RSP_BAN_AFTER_OF_TK", 3, 3, 3, 3, 0, 99},
	{"vvsa", "ID_RSP_VEHICLES", 1, 1, 0, 1, 0, 1},
	{"vrhe", "ID_RSP_REGEN_HEALTH", 1, 0, 1, 1, 0, 1},
	{"osls", "ID_RSP_SQUAD_LEADER_SPAWN", 0, 1, 0, 1, 0, 1},
	{"vmin", "ID_RSP_MINIMAP", 1, 1, 1, 1, 0, 1},
	{"vhud", "ID_RSP_HUD", 1, 0, 1, 1, 0, 1},
	{"vmsp", "ID_RSP_MINIMAP_SPOTTING", 1, 0, 1, 1, 0, 1},
	{"v3sp", "ID_RSP_3D_SPOTTING", 1, 0, 1, 0, 0, 1},
	{"vkca", "ID_RSP_KILL_CAM", 1, 0, 1, 0, 0, 1},
	{"v3ca", "ID_RSP_3P_VEHICLE_CAM", 1, 0, 1, 0, 0, 1},
	{"vnta", "ID_RSP_NAMETAG", 1, 0, 1, 1, 0, 1},
	{"vprt", "ID_RSP_PLAYER_RESPAWN", 100, 100, 100, 100, 0, 300},
	{"vpmd", "ID_RSP_PLAYER_MAN_DOWN", 100, 100, 100, 100, 0, 300},
	{"vshe", "ID_RSP_PLAYER_HEALTH", 100, 60, 100, 100, 1, 300},
	{"vbdm", "ID_RSP_BULLET_MODIFIER", 100, 100, 100, 100, 1, 300},
	{"vhie", "ID_RSP_HIT_INDICATOR", 1, 0, 1, 1, 0, 1},
	{"frwm", "ID_RSP_RELOAD_WHOLE_MAGS", 0, 1, 0, 0, 0, 1},
	{"vcom", "ID_RSP_ALLOW_COMMANDERS", 1, 1, 1, 1, 0, 1},
	{"vaas", "ID_RSP_ALLOW_PUBLIC_SPECTATORS", 1, 1, 1, 1, 0, 1},
}

var (
	RspPingSiteAlias = "rs-prod-ps3"
	RspPingSiteName  = "Europe"
)

func rspPresetList() []rspPreset {
	mk := func(pid uint32, name string, pick func(s rspSetting) uint32) rspPreset {
		p := rspPreset{DESC: "ID_RSP_PRESET_" + name, NAME: name, PID: pid, PTYP: name, STYP: 1}
		for _, s := range rspSettings {
			p.SLST = append(p.SLST, rspPresetSetting{KEY: s.key, VAL: pick(s)})
		}
		return p
	}
	return []rspPreset{
		mk(1, "NORMAL", func(s rspSetting) uint32 { return s.normal }),
		mk(2, "HARDCORE", func(s rspSetting) uint32 { return s.hard }),
		mk(3, "INFANTRY", func(s rspSetting) uint32 { return s.infantry }),
		mk(4, "CLASSIC", func(s rspSetting) uint32 { return s.clas }),
	}
}

func rspConfig() rspGetConfigResponse {
	r := rspGetConfigResponse{
		ACFG: rspAdminConfig{MRMX: 20, SAMX: 25, SBMX: 100, SVMX: 25, UAMX: 25, UOMX: 25},
		PCAT: []rspSettingCategory{{CID: 1, NAME: "ID_RSP_CATEGORY_GENERAL"}},
		PLST: rspPresetList(),
		PSLS: []rspPingSite{{ALIA: RspPingSiteAlias, CAP: 64, NAME: RspPingSiteName}},
	}
	for _, s := range rspSettings {
		r.PSET = append(r.PSET, rspSettingConfig{CID: 1, DESC: s.name + "_DESC", KEY: s.key, NAME: s.name})
		r.PVAL = append(r.PVAL, rspSettingRange{KEY: s.key, MAX: s.max, MIN: s.min})
	}
	return r
}

func handleRspGetConfig(p blaze.Packet) []byte {
	cfg := rspConfig()
	logger.Info("0801: RSP getConfig -> %d presets, %d settings, ping site %q = %q",
		len(cfg.PLST), len(cfg.PSET), RspPingSiteAlias, RspPingSiteName)
	return encodeReply(p, &cfg, "RSP getConfig")
}
