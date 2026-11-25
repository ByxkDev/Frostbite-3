package ebx

import (
	"path/filepath"
	"testing"
)

func TestGUIDParseRoundTrip(t *testing.T) {
	cases := []string{
		"bf2b55cf-6b8e-3119-e34b-08520f666727",
		"bf2b55cf6b8e3119e34b08520f666727",
		"BF2B55CF-6B8E-3119-E34B-08520F666727",
	}
	want := "bf2b55cf-6b8e-3119-e34b-08520f666727"
	for _, in := range cases {
		g, err := ParseGUID(in)
		if err != nil {
			t.Fatalf("ParseGUID(%q): %v", in, err)
		}
		if g.String() != want {
			t.Errorf("ParseGUID(%q).String() = %q, want %q", in, g.String(), want)
		}
	}
	if _, err := ParseGUID("not-a-guid"); err == nil {
		t.Error("ParseGUID(bad) = nil error, want error")
	}
}

func TestDeriveGUIDStable(t *testing.T) {
	a := DeriveGUID("partition/Levels/MP/MP_Siege/MP_Siege")
	b := DeriveGUID("partition/Levels/MP/MP_Siege/MP_Siege")
	if a != b {
		t.Errorf("DeriveGUID not stable: %s != %s", a, b)
	}
	if a == DeriveGUID("partition/Levels/MP/MP_Naval/MP_Naval") {
		t.Error("DeriveGUID collided for different seeds")
	}
	if a.IsZero() {
		t.Error("DeriveGUID returned zero")
	}
}

func TestPartitionPrimaryAndLookup(t *testing.T) {
	p := NewPartition("Weapons/AK74M/AK74", Zero)
	if p.GUID.IsZero() {
		t.Fatal("NewPartition left a zero GUID")
	}
	blueprint := p.Add(NewInstance("SoldierBlueprint", DeriveGUID("bp")), false)
	data := p.Add(NewInstance("SoldierEntityData", DeriveGUID("data")).Set("MaxHealth", 100.0), false)

	if p.Primary() != blueprint {
		t.Error("first added instance should be primary")
	}

	p.Add(data, true)
	if p.Primary() != data {
		t.Error("promoted instance should be primary")
	}
	if blueprint.Primary {
		t.Error("old primary should have been demoted")
	}
	if got := p.FindByType("soldierentitydata"); got != data {
		t.Error("FindByType is not case-insensitive / wrong instance")
	}
	if hp, ok := data.Float("MaxHealth"); !ok || hp != 100.0 {
		t.Errorf("MaxHealth = %v, %v; want 100, true", hp, ok)
	}
}

func TestDBResolveRef(t *testing.T) {
	db := NewDB()
	soldier := NewPartition("Characters/MpSoldier/MpSoldier", Zero)
	entity := soldier.Add(NewInstance("SoldierEntityData", DeriveGUID("se")), true)
	db.Add(soldier)

	team := NewPartition("Gameplay/Team", Zero)
	team.Add(NewInstance("TeamData", DeriveGUID("td")).Set("Soldier", soldier.Ref(entity)), true)
	db.Add(team)

	r, _ := team.Primary().RefField("Soldier")
	got, ok := db.Resolve(team, r)
	if !ok || got != entity {
		t.Fatalf("Resolve external ref failed: ok=%v got=%v", ok, got)
	}
	if _, ok := db.Partition("characters/mpsoldier/mpsoldier"); !ok {
		t.Error("DB.Partition should be case-insensitive")
	}
	if _, ok := db.ByGUID(soldier.GUID); !ok {
		t.Error("DB.ByGUID missing a registered partition")
	}
}

func TestDefaultCatalogBuildAndSetup(t *testing.T) {
	w := DefaultCatalog().Build()

	if w.DB.Len() != 90 {
		t.Errorf("DB.Len() = %d, want 90", w.DB.Len())
	}

	setup, err := w.Setup("levels/mp/mp_siege/mp_siege", "teamdeathmatch0")
	if err != nil {
		t.Fatalf("Setup (case-insensitive) failed: %v", err)
	}
	if setup.Bundle != "Levels/MP/MP_Siege/TeamDeathMatch" {
		t.Errorf("Bundle = %q, want Levels/MP/MP_Siege/TeamDeathMatch", setup.Bundle)
	}
	if setup.LevelData == nil || setup.LevelData.Type != "LevelData" {
		t.Error("LevelData primary instance missing or wrong type")
	}
	if setup.SubWorldDat == nil || setup.SubWorldDat.Type != "SubWorldData" {
		t.Error("SubWorldData primary instance missing or wrong type")
	}
	if setup.LevelGUID() == setup.ModeGUID() {
		t.Error("level and mode partitions should have distinct GUIDs")
	}

	subs, ok := setup.LevelData.Fields["SubWorlds"].([]any)
	if !ok || len(subs) != 8 {
		t.Fatalf("SubWorlds = %v, want 8 entries", setup.LevelData.Fields["SubWorlds"])
	}
	first := subs[0].(map[string]any)
	ref := first["Ref"].(Ref)
	if _, ok := w.DB.Resolve(setup.LevelPart, ref); !ok {
		t.Error("a SubWorld Ref did not resolve through the DB")
	}

	if _, err := w.Setup("Levels/MP/MP_Siege/MP_Siege", "NoSuchMode"); err == nil {
		t.Error("Setup with bad mode should error")
	}
	if _, err := w.Setup("Levels/MP/Nope/Nope", "TeamDeathMatch0"); err == nil {
		t.Error("Setup with bad level should error")
	}
}

func TestLoadWritesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "levels.json")

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load(missing): %v", err)
	}
	if len(c.Levels) == 0 {
		t.Fatal("Load(missing) returned an empty catalog")
	}

	c2, err := Load(path)
	if err != nil {
		t.Fatalf("Load(existing): %v", err)
	}
	if len(c2.Levels) != len(c.Levels) {
		t.Errorf("reloaded catalog has %d levels, want %d", len(c2.Levels), len(c.Levels))
	}
}

func TestGUIDOverrideFromCatalog(t *testing.T) {
	real := MustGUID("11112222-3333-4444-5555-666677778888")
	c := &Catalog{Levels: []Level{{
		LevelName: "Levels/MP/MP_Siege/MP_Siege",
		Display:   "Siege of Shanghai",
		GUID:      &real,
		Modes:     []GameMode{{Key: "TeamDeathMatch0", Name: "Team Deathmatch"}},
	}}}
	w := c.Build()
	p, ok := w.DB.Partition("Levels/MP/MP_Siege/MP_Siege")
	if !ok {
		t.Fatal("level partition not built")
	}
	if p.GUID != real {
		t.Errorf("level GUID = %s, want the overridden %s", p.GUID, real)
	}
}
