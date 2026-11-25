package ebx

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

type GameMode struct {
	Key  string `json:"key"`
	Name string `json:"name"`

	GUID *GUID `json:"guid,omitempty"`
}

type Level struct {
	LevelName string     `json:"levelName"`
	Display   string     `json:"display"`
	Code      string     `json:"code,omitempty"`
	Modes     []GameMode `json:"modes"`

	GUID *GUID `json:"guid,omitempty"`

	MaxVehicleFlightHeight float64 `json:"maxVehicleFlightHeight,omitempty"`
}

type Catalog struct {
	Levels []Level `json:"levels"`
}

func (c *Catalog) Level(levelName string) (*Level, bool) {
	for i := range c.Levels {
		if strings.EqualFold(c.Levels[i].LevelName, levelName) {
			return &c.Levels[i], true
		}
	}
	return nil, false
}

func (l *Level) Mode(key string) (*GameMode, bool) {
	for i := range l.Modes {
		if strings.EqualFold(l.Modes[i].Key, key) {
			return &l.Modes[i], true
		}
	}
	return nil, false
}

func (c *Catalog) LevelNames() []string {
	out := make([]string, 0, len(c.Levels))
	for _, l := range c.Levels {
		out = append(out, l.LevelName)
	}
	slices.Sort(out)
	return out
}

func modeBase(key string) string {
	return strings.TrimRight(key, "0123456789")
}

func bundleName(levelName, modeKey string) string {
	return path.Dir(levelName) + "/" + modeBase(modeKey)
}

func gamemodePartitionPath(levelName, modeKey string) string {
	return path.Dir(levelName) + "/" + strings.ToUpper(modeBase(modeKey))
}

type GameSetup struct {
	LevelName   string
	ModeKey     string
	ModeName    string
	Bundle      string
	LevelPart   *Partition
	LevelData   *Instance
	ModePart    *Partition
	SubWorldDat *Instance
}

func (s *GameSetup) LevelGUID() GUID { return s.LevelPart.GUID }

func (s *GameSetup) ModeGUID() GUID { return s.ModePart.GUID }

func (s *GameSetup) String() string {
	return fmt.Sprintf("%s [%s] level=%s mode=%s bundle=%q",
		s.LevelName, s.ModeName, s.LevelPart.GUID, s.ModePart.GUID, s.Bundle)
}

type World struct {
	Catalog *Catalog
	DB      *DB
}

func (c *Catalog) Build() *World {
	db := NewDB()

	for li := range c.Levels {
		l := &c.Levels[li]

		levelPart := NewPartition(l.LevelName, guidOrZero(l.GUID))
		levelData := NewInstance("LevelData", DeriveGUID("instance/"+l.LevelName+"/LevelData"))
		levelData.Set("Name", l.LevelName)
		levelData.Set("DisplayName", l.Display)
		if l.Code != "" {
			levelData.Set("Code", l.Code)
		}
		if l.MaxVehicleFlightHeight != 0 {
			levelData.Set("MaxVehicleFlightHeight", l.MaxVehicleFlightHeight)
		}
		levelPart.Add(levelData, true)

		var subRefs []any
		for mi := range l.Modes {
			m := &l.Modes[mi]

			modePath := gamemodePartitionPath(l.LevelName, m.Key)
			modePart := NewPartition(modePath, guidOrZero(m.GUID))
			subWorld := NewInstance("SubWorldData", DeriveGUID("instance/"+modePath+"/SubWorldData"))
			subWorld.Set("Mode", m.Key)
			subWorld.Set("DisplayName", m.Name)
			subWorld.Set("Bundle", bundleName(l.LevelName, m.Key))
			subWorld.Set("Level", levelPart.Ref(levelData))
			modePart.Add(subWorld, true)
			db.Add(modePart)

			subRefs = append(subRefs, map[string]any{
				"Mode": m.Key,
				"Ref":  modePart.Ref(subWorld),
			})
		}
		levelData.Set("SubWorlds", subRefs)
		db.Add(levelPart)
	}

	return &World{Catalog: c, DB: db}
}

func (w *World) Setup(levelName, modeKey string) (*GameSetup, error) {
	l, ok := w.Catalog.Level(levelName)
	if !ok {
		return nil, fmt.Errorf("ebx: unknown level %q (known: %s)",
			levelName, strings.Join(w.Catalog.LevelNames(), ", "))
	}
	m, ok := l.Mode(modeKey)
	if !ok {
		return nil, fmt.Errorf("ebx: level %q has no mode %q (modes: %s)",
			l.LevelName, modeKey, strings.Join(l.ModeKeys(), ", "))
	}

	levelPart, _ := w.DB.Partition(l.LevelName)
	modePart, _ := w.DB.Partition(gamemodePartitionPath(l.LevelName, m.Key))
	if levelPart == nil || modePart == nil {
		return nil, fmt.Errorf("ebx: partitions for %q/%q were not built", l.LevelName, m.Key)
	}

	return &GameSetup{
		LevelName:   l.LevelName,
		ModeKey:     m.Key,
		ModeName:    m.Name,
		Bundle:      bundleName(l.LevelName, m.Key),
		LevelPart:   levelPart,
		LevelData:   levelPart.Primary(),
		ModePart:    modePart,
		SubWorldDat: modePart.Primary(),
	}, nil
}

func guidOrZero(g *GUID) GUID {
	if g == nil {
		return Zero
	}
	return *g
}

func (l *Level) ModeKeys() []string {
	out := make([]string, len(l.Modes))
	for i, m := range l.Modes {
		out[i] = m.Key
	}
	return out
}
