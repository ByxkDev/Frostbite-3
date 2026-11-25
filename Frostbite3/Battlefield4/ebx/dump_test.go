package ebx

import (
	"strings"
	"testing"
)

const dumpSample = "c028d4ed-2bbc-42f0-9ae4-e008b9cdd265\r\n" +
	"WorldPartData 4f3de3d6-2a02-4b68-85d1-4a9845e653f2\r\n" +
	"\t$::Asset\r\n" +
	"\t\t$::DataContainer\r\n" +
	"\t\tName Levels/MP/MP_Siege/Deathmatch_Common\r\n" +
	"\tObjects::array\r\n" +
	"\t\tmember(0) Instance0\r\n" +
	"AlternateSpawnEntityData 0c7e62f4-b9d4-4ee0-b2e5-646cf9d3e519\r\n" +
	"\t$::SpatialEntityData\r\n" +
	"\t\t$::EntityData\r\n" +
	"\t\t\t$::GameObjectData\r\n" +
	"\t\t\t\t$::DataBusPeer\r\n" +
	"\t\t\t\t\t$::GameDataContainer\r\n" +
	"\t\t\t\t\t\t$::DataContainer\r\n" +
	"\t\t\t\t\tFlags 100663296\r\n" +
	"\t\tTransform::LinearTransform\r\n" +
	"\t\t\tright::Vec3\r\n\t\t\t\tx -0.38\r\n\t\t\t\ty 0.0\r\n\t\t\t\tz -0.92\r\n" +
	"\t\t\tup::Vec3\r\n\t\t\t\tx 0.0\r\n\t\t\t\ty 1.0\r\n\t\t\t\tz 0.0\r\n" +
	"\t\t\tforward::Vec3\r\n\t\t\t\tx 0.92\r\n\t\t\t\ty 0.0\r\n\t\t\t\tz -0.38\r\n" +
	"\t\t\ttrans::Vec3\r\n\t\t\t\tx 159.33\r\n\t\t\t\ty 66.43\r\n\t\t\t\tz -84.71\r\n" +
	"\tTeam TeamNeutral\r\n" +
	"\tPriority 2.0\r\n" +
	"\tEnabled True\r\n" +
	"VolumeVectorShapeData Instance0\r\n" +
	"\tPoints::array\r\n\t\tmember(0)::Vec3\r\n\t\t\tx 1.0\r\n\t\t\ty 2.0\r\n\t\t\tz 3.0\r\n"

func TestParseDump(t *testing.T) {
	p, err := ParseDump(strings.NewReader(dumpSample))
	if err != nil {
		t.Fatal(err)
	}
	if p.GUID != "c028d4ed-2bbc-42f0-9ae4-e008b9cdd265" || p.Name != "Levels/MP/MP_Siege/Deathmatch_Common" {
		t.Fatalf("partition %q %q", p.GUID, p.Name)
	}
	if len(p.Instances) != 3 {
		t.Fatalf("%d instances", len(p.Instances))
	}
	sp := p.Get("0C7E62F4-B9D4-4EE0-B2E5-646CF9D3E519")
	if sp == nil || !sp.Is("GameObjectData") || sp.Field("Flags") != "100663296" || sp.Field("Team") != "TeamNeutral" {
		t.Fatalf("spawn %+v", sp)
	}
	tr, ok := sp.Transform("Transform")
	if !ok || tr.Trans != [3]float64{159.33, 66.43, -84.71} || tr.Forward[0] != 0.92 {
		t.Fatalf("transform %+v %v", tr, ok)
	}
	if v, ok := p.Get("Instance0").Vec3("Points.0"); !ok || v != [3]float64{1, 2, 3} {
		t.Fatalf("array vec %v %v", v, ok)
	}
	s := NewStore()
	s.Add(p)
	if s.Resolve(p.Primary(), "Instance0") == nil || s.Resolve(nil, "Levels/MP/MP_Siege/Deathmatch_Common/0c7e62f4-b9d4-4ee0-b2e5-646cf9d3e519") != sp {
		t.Fatal("resolve")
	}
	if got := s.AlternateSpawns("levels/mp/mp_siege/deathmatch_common"); len(got) != 1 || got[0].Priority != 2 || !got[0].Enabled {
		t.Fatalf("spawns %+v", got)
	}
}

func TestLoadRealDump(t *testing.T) {
	s, err := LoadDumpDirs("../"+DefaultDumpDir, "../"+DefaultSbTocDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.PartitionNames()) == 0 {
		t.Skip("no dump shipped")
	}
	if n := len(s.PartitionNames()); n < 250 {
		t.Fatalf("only %d partitions", n)
	}
	sp := s.AlternateSpawns("Levels/MP/MP_Siege/Deathmatch_Common")
	if len(sp) != 21 {
		t.Fatalf("%d TDM spawns, want 21", len(sp))
	}
	found := false
	for _, p := range sp {
		if p.Transform.Trans[0] > 159.3 && p.Transform.Trans[0] < 159.4 && p.Transform.Trans[1] > 66 {
			found = true
		}
	}
	if !found {
		t.Fatal("spawn (159.33, 66.43, -84.71) missing")
	}
	reg, err := LoadNetworkRegistryDir("../" + DefaultRegistryDir)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(reg.PartitionNames()); n != 112 {
		t.Fatalf("%d registry partitions, want 112 (15 public + 97 from the .sb files)", n)
	}
	if reg.Partition("levels/mp/mp_siege/gunmaster_networkregistry_Ps3") == nil {
		t.Fatal("GunMaster registry from the .sb files missing")
	}
	sb := s.Partition("Gameplay/Soldiers/MpSoldier")
	if sb == nil || sb.Primary().Type != "SoldierBlueprint" || sb.Primary().ID != "b70db8c3-c043-41e4-971d-c997970d35a5" {
		t.Fatalf("MpSoldier from the .sb dump: %+v", sb)
	}
	tdm := reg.Partition("levels/mp/mp_siege/teamdeathmatch0_networkregistry_Ps3")
	if tdm == nil {
		t.Fatal("no TDM registry")
	}
	typed := 0
	for _, o := range tdm.Objects {
		if s.RegistryType(o) != "" {
			typed++
		}
	}
	if typed == 0 {
		t.Fatal("no TDM registry object found in the dump")
	}
	t.Logf("%s; TDM registry %d objects, %d with a type in the dump", s, len(tdm.Objects), typed)
}
