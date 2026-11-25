package components

import (
	"bytes"
	"testing"
)

func tdfTag(t string) []byte {
	var v uint32
	for i := range 4 {
		c := byte(' ')
		if i < len(t) {
			c = t[i]
		}
		v = v<<6 | uint32(c-32)&63
	}
	return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
}

func TestRspGetConfigEncodes(t *testing.T) {
	cfg := rspConfig()
	b, err := component35TdfEncoder.Encode(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"ACFG", "PCAT", "PLST", "PSET", "PSLS", "PVAL", "ALIA", "NAME", "SLST", "STYP"} {
		if !bytes.Contains(b, tdfTag(tag)) {
			t.Fatalf("tag %s missing", tag)
		}
	}
	if !bytes.Contains(b, []byte("NORMAL\x00")) || !bytes.Contains(b, []byte("rs-prod-ps3\x00")) {
		t.Fatal("preset / ping site strings missing")
	}
	if len(cfg.PLST) != 4 || len(cfg.PLST[0].SLST) != len(rspSettings) {
		t.Fatal("presets")
	}
}
