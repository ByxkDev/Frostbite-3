package components

import (
	"os"
	"path/filepath"
	"testing"

	"bf4/blaze"
)

func TestGetPacksIsCapped(t *testing.T) {
	dir := t.TempDir()
	oldDir, oldFile := PacksDir, BattlepacksFile
	PacksDir = dir
	BattlepacksFile = filepath.Join(dir, "none.txt")
	defer func() { PacksDir, BattlepacksFile = oldDir, oldFile }()
	var st packStore
	for i := 1; i <= 300; i++ {
		st.Packs = append(st.Packs, battlepack{ID: uint64(i), Key: "bronze_pack", Items: []string{}})
	}
	if err := savePacks(currentPersonaID(), st); err != nil {
		t.Fatal(err)
	}
	out := HandlePacks(blaze.Packet{Component: Packs, Command: packsGetPacks, MessageId: 1})
	if len(out) == 0 {
		t.Fatal("no reply")
	}
	if n := countPackIDs(out); n > int(MaxPacksListed) {
		t.Fatalf("%d packs listed", n)
	}
	_ = os.Remove(packsPath(currentPersonaID()))
}

func countPackIDs(b []byte) int {
	tag := blaze.MustTdfMember("PID").Bytes
	n := 0
	for i := 0; i+3 <= len(b); i++ {
		if b[i] == tag[0] && b[i+1] == tag[1] && b[i+2] == tag[2] {
			n++
		}
	}
	return n
}
