package ebx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultCatalogFile = "data/ebx/levels.json"

func standardModes() []GameMode {
	return []GameMode{
		{Key: "ConquestLarge0", Name: "Conquest Large"},
		{Key: "ConquestSmall0", Name: "Conquest"},
		{Key: "Domination0", Name: "Domination"},
		{Key: "Elimination0", Name: "Defuse"},
		{Key: "Obliteration", Name: "Obliteration"},
		{Key: "RushLarge0", Name: "Rush"},
		{Key: "SquadDeathMatch0", Name: "Squad Deathmatch"},
		{Key: "TeamDeathMatch0", Name: "Team Deathmatch"},
	}
}

func DefaultCatalog() *Catalog {
	type m struct{ code, display string }
	base := []m{
		{"MP_Abandoned", "Zavod 311"},
		{"MP_Damage", "Lancang Dam"},
		{"MP_Flooded", "Flood Zone"},
		{"MP_Journey", "Golmud Railway"},
		{"MP_Naval", "Paracel Storm"},
		{"MP_Prison", "Operation Locker"},
		{"MP_Resort", "Hainan Resort"},
		{"MP_Siege", "Siege of Shanghai"},
		{"MP_TheDish", "Rogue Transmission"},
		{"MP_Tremors", "Dawnbreaker"},
	}
	c := &Catalog{}
	for _, b := range base {
		c.Levels = append(c.Levels, Level{
			LevelName: fmt.Sprintf("Levels/MP/%s/%s", b.code, b.code),
			Display:   b.display,
			Code:      b.code,
			Modes:     standardModes(),
		})
	}
	return c
}

func Load(path string) (*Catalog, error) {
	b, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		c := DefaultCatalog()
		if werr := Save(path, c); werr != nil {
			return c, nil
		}
		return c, nil
	case err != nil:
		return nil, fmt.Errorf("ebx: read %s: %w", path, err)
	}

	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("ebx: parse %s: %w", path, err)
	}
	if len(c.Levels) == 0 {
		return nil, fmt.Errorf("ebx: %s has no levels", path)
	}
	return &c, nil
}

func Save(path string, c *Catalog) error {
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func LoadWorld(path string) (*World, error) {
	c, err := Load(path)
	if err != nil {
		return nil, err
	}
	return c.Build(), nil
}
