package ebx

import (
	"strings"
	"testing"
)

const tdmRegistrySample = `710b2378-0aac-dd1c-f155-6aa53e09564e
NetworkRegistryAsset 710b2378-0aac-dd1c-f155-6aa53e09564e
	$::Asset
		$::DataContainer
		Name levels/mp/mp_siege/teamdeathmatch0_networkregistry_Ps3
	Objects::array
		member(0) Gameplay/GameModes/DeathMatch/Components/DEATHMATCH/cf513300-8403-4715-b6ed-cff1d5d289b6
		member(1) Gameplay/GameModes/DeathMatch/Components/DEATHMATCH/f75f6503-a8e7-4987-b7a7-6bc128b15f09
		member(4) Levels/MP/MP_Siege/TeamDeathmatch0_Logic/f3d86110-2d73-44f4-8774-f051b5c2eb5b
		member(5) UI/Flow/Logic/UIGameLogicMP/fdefff10-1c21-a7de-df24-b5a601d8b965
		member(7) UI/Flow/Logic/UISpawnMenuMPLogic/43e0e31e-941f-6abe-5ec7-92692d6d800c
`

func TestParseNetworkRegistry(t *testing.T) {
	p, err := ParseNetworkRegistry(strings.NewReader(tdmRegistrySample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Name != "levels/mp/mp_siege/teamdeathmatch0_networkregistry_Ps3" {
		t.Fatalf("Name = %q", p.Name)
	}
	if len(p.Objects) != 5 {
		t.Fatalf("got %d objects, want 5", len(p.Objects))
	}

	spawn, ok := p.Find("UISpawnMenuMPLogic")
	if !ok || spawn.Index != 4 {
		t.Errorf("UISpawnMenuMPLogic at index %d (ok=%v), want 4", spawn.Index, ok)
	}
	gl, ok := p.Find("UIGameLogicMP")
	if !ok || gl.Index != 3 {
		t.Errorf("UIGameLogicMP at index %d (ok=%v), want 3", gl.Index, ok)
	}
}

func TestNetworkRegistryIndices(t *testing.T) {
	r := NewNetworkRegistry()
	p, err := ParseNetworkRegistry(strings.NewReader(tdmRegistrySample))
	if err != nil {
		t.Fatal(err)
	}
	r.AddPartition(p)
	part := p.Name

	logic := MustGUID("f3d86110-2d73-44f4-8774-f051b5c2eb5b")
	idx, total, ok := r.DataRefIndex(part, logic)
	if !ok || idx != 2 || total != 5 {
		t.Errorf("DataRefIndex(logic) = %d,%d,%v; want 2,5,true", idx, total, ok)
	}

	if oi, ok := r.OwnerRefIndex(part, logic); !ok || oi != 3 {
		t.Errorf("OwnerRefIndex(logic) = %d,%v; want 3,true", oi, ok)
	}
	if oi, ok := r.OwnerRefIndex(part, Zero); !ok || oi != 0 {
		t.Errorf("OwnerRefIndex(null) = %d,%v; want 0,true", oi, ok)
	}
	if _, _, ok := r.DataRefIndex(part, DeriveGUID("absent")); ok {
		t.Errorf("absent object should be ok=false")
	}
	if p := DataRefPartition(3); p != 0 {
		t.Errorf("DataRefPartition(3) = %d, want 0", p)
	}
}

func TestLoadRealRegistryDir(t *testing.T) {
	r, err := LoadNetworkRegistryDir("../data/ebx/registry")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	tdm := r.Partition("levels/mp/mp_siege/teamdeathmatch0_networkregistry_Ps3")
	if tdm == nil {
		t.Skip("registry dumps not present")
	}
	if got := len(tdm.Objects); got < 70 {
		t.Errorf("TDM registry has %d objects, want >=70", got)
	}
	for _, name := range []string{"UISpawnMenuMPLogic", "UIGameLogicMP", "TeamDeathmatch0_Logic"} {
		if _, ok := tdm.Find(name); !ok {
			t.Errorf("TDM registry missing %s", name)
		}
	}
}
