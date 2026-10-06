package server

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"bf4/blaze"
	"bf4/logger"
)

const (
	LocaleEnUS = 1701729619
	PingSite = "rs-prod-ps3"
	ProtocolVersion = "5900"
	DefaultGamePort = 25200
	DefaultSettings = 0x52F
)

const SiegeLevel = "Levels/MP/MP_Siege/MP_Siege"

var SiegeModes = map[string]string{
	"ConquestLarge0":     "Conquest Large",
	"ConquestSmall0":     "Conquest",
	"Domination0":        "Domination",
	"Elimination0":       "Defuse",
	"GunMaster0":         "Gun Master",
	"Obliteration":       "Obliteration",
	"RushLarge0":         "Rush",
	"SquadDeathMatch0":   "Squad Deathmatch",
	"SquadObliteration0": "Squad Obliteration",
	"TeamDeathMatch0":    "Team Deathmatch",
}

type GameConfig struct {
	Name       string            `json:"name"`
	Level      string            `json:"level"`
	Mode       string            `json:"mode"`
	MaxPlayers uint16            `json:"maxPlayers"`
	HostIP     string            `json:"hostIP"`
	HostPort   uint16            `json:"hostPort"`
	Settings   uint32            `json:"settings,omitempty"`
	Version    string            `json:"version,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

var GamesFile = "data/games.json"

func DefaultGames(hostIP string) []GameConfig {
	return []GameConfig{{
		Name:       "[EMU] Siege of Shanghai - Team Deathmatch",
		Level:      SiegeLevel,
		Mode:       "TeamDeathMatch0",
		MaxPlayers: 24,
		HostIP:     hostIP,
		HostPort:   DefaultGamePort,
		Attributes: map[string]string{"mod": "DEFAULT"},
	}}
}

func LoadGames(r *Registry, hostIP string) error {
	var cfgs []GameConfig

	b, err := os.ReadFile(GamesFile)
	switch {
	case os.IsNotExist(err):
		cfgs = DefaultGames(hostIP)
		if out, mErr := json.MarshalIndent(cfgs, "", "  "); mErr == nil {
			if dErr := os.MkdirAll(dirOf(GamesFile), 0o755); dErr == nil {
				if wErr := os.WriteFile(GamesFile, out, 0o644); wErr == nil {
					logger.Info("SERVER: wrote default %s", GamesFile)
				}
			}
		}
	case err != nil:
		return fmt.Errorf("read %s: %w", GamesFile, err)
	default:
		if err := json.Unmarshal(b, &cfgs); err != nil {
			return fmt.Errorf("parse %s: %w", GamesFile, err)
		}
	}

	for _, c := range cfgs {
		g, err := c.toGame(hostIP)
		if err != nil {
			logger.Warn("SERVER: skipping %q: %v", c.Name, err)
			continue
		}
		r.Add(g)
		logger.Info("SERVER: listed game %d %q level=%s mode=%s max=%d host=%s:%d",
			g.ID, g.Name, g.Level, g.Mode, g.MaxPlayers, ipString(g.HostIP), g.HostPort)
	}
	return nil
}

func (c GameConfig) toGame(defaultIP string) (*Game, error) {
	if c.Level == "" {
		c.Level = SiegeLevel
	}
	if c.Mode == "" {
		return nil, fmt.Errorf("no mode")
	}
	if strings.EqualFold(c.Level, SiegeLevel) {
		if _, ok := SiegeModes[c.Mode]; !ok {
			logger.Warn("SERVER: %q: mode %q isn't one of MP_Siege's modes", c.Name, c.Mode)
		}
	}
	if c.MaxPlayers == 0 {
		c.MaxPlayers = 24
	}
	if c.HostIP == "" {
		c.HostIP = defaultIP
	}
	if c.HostPort == 0 {
		c.HostPort = DefaultGamePort
	}
	if c.Settings == 0 {
		c.Settings = DefaultSettings
	}
	if c.Version == "" {
		c.Version = ProtocolVersion
	}

	ip, err := parseIPv4(c.HostIP)
	if err != nil {
		return nil, err
	}

	return &Game{
		Name:       c.Name,
		Level:      c.Level,
		Mode:       c.Mode,
		Attributes: copyAttrs(c.Attributes),
		MaxPlayers: c.MaxPlayers,
		Settings:   c.Settings,
		State:      GameStateInGame,
		Topology:   TopologyDedicated,
		Version:    c.Version,
		HostIP:     ip,
		HostPort:   c.HostPort,
		Static:     true,
	}, nil
}

func parseIPv4(s string) (uint32, error) {
	ip := net.ParseIP(strings.TrimSpace(s)).To4()
	if ip == nil {
		return 0, fmt.Errorf("bad IPv4 address %q", s)
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3]), nil
}

func ipString(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}

func dirOf(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[:i]
	}
	return "."
}

func blazeUnionSet(u *NetworkAddress, pair *IpPairAddress) error {
	return blaze.UnionSetValue(u, pair)
}