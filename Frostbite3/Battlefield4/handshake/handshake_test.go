package handshake

import (
	"crypto/sha256"
	"strings"
	"testing"

	"bf4/ebx"
)

func fakeClient(counter byte, ctx map[string]string) []byte {
	pt := make([]byte, BodyLen)
	pt[0], pt[1], pt[2], pt[3] = 0xB4, 0x0F, 0x00, counter
	g := ebx.MustGUID(ctx["blaze.uuid"])
	copy(pt[4:20], g[:])
	port := 25210
	if ctx["server.port"] == "25211" {
		port = 25211
	}
	pt[20], pt[21] = byte(port>>8), byte(port)

	ks := sha256.Sum256([]byte("secret|" + string(rune(counter)) + "|" + ctx["blaze.seed"]))
	pkt := []byte{counter, RequestFlag}
	for i := range pt {
		pkt = append(pkt, pt[i]^ks[i])
	}
	return pkt
}

func baseCtx() map[string]string {
	return map[string]string{
		"blaze.uuid":  "7c9e6679-7425-40de-944b-e07fc1f90ae7",
		"blaze.seed":  "11181",
		"blaze.gsid":  "1337",
		"server.port": "25210",
	}
}

func with(ctx map[string]string, k, v string) map[string]string {
	out := map[string]string{}
	for a, b := range ctx {
		out[a] = b
	}
	out[k] = v
	return out
}

func rec(t *testing.T, counter byte, ctx map[string]string) Record {
	r, ok := NewRecord("1.2.3.4:3659", fakeClient(counter, ctx), ctx)
	if !ok {
		t.Fatal("NewRecord rejected a valid request")
	}
	return r
}

func find(fs []Finding, k Kind, field string) *Finding {
	for i := range fs {
		if fs[i].Kind == k && fs[i].Field == field {
			return &fs[i]
		}
	}
	return nil
}

func TestParse(t *testing.T) {
	if _, _, _, ok := Parse(make([]byte, 23)); ok {
		t.Error("23 bytes accepted")
	}
	pkt := make([]byte, 24)
	pkt[0], pkt[1] = 9, 0x80
	c, f, body, ok := Parse(pkt)
	if !ok || c != 9 || f != 0x80 || len(body) != 22 {
		t.Errorf("Parse = %d %x %d %v", c, f, len(body), ok)
	}
}

func TestUUIDFoundInPlaintext(t *testing.T) {
	a := rec(t, 0, baseCtx())
	b := rec(t, 0, with(baseCtx(), "blaze.uuid", "f47ac10b-58cc-4372-a567-0e02b2c3d479"))
	fs := ComparePair(a, b)
	pt := find(fs, Plaintext, "blaze.uuid")
	if pt == nil {
		t.Fatalf("UUID not located; findings: %v", fs)
	}
	if pt.Offset != 4 || pt.Encoding != "guid-raw" || !pt.Clean {
		t.Errorf("got offset=%d enc=%s clean=%v, want 4 guid-raw clean", pt.Offset, pt.Encoding, pt.Clean)
	}
	if find(fs, Input, "blaze.uuid") == nil {
		t.Error("single-field change should also be reported as INPUT")
	}
}

func TestPortFoundInPlaintext(t *testing.T) {
	a := rec(t, 3, baseCtx())
	b := rec(t, 3, with(baseCtx(), "server.port", "25211"))
	pt := find(ComparePair(a, b), Plaintext, "server.port")
	if pt == nil || pt.Offset != 20 || pt.Encoding != "u16be" {
		t.Fatalf("port finding = %+v, want offset 20 u16be", pt)
	}
}

func TestKeyMaterialIsDiffused(t *testing.T) {
	a := rec(t, 0, baseCtx())
	b := rec(t, 0, with(baseCtx(), "blaze.seed", "424242"))
	fs := ComparePair(a, b)
	in := find(fs, Input, "blaze.seed")
	if in == nil || !strings.Contains(in.Detail, "diffused") {
		t.Fatalf("seed should be a diffused INPUT, got %v", fs)
	}
	if find(fs, Plaintext, "blaze.seed") != nil {
		t.Error("key material must not be reported as plaintext")
	}
}

func TestExcludedAndHidden(t *testing.T) {
	a := rec(t, 0, baseCtx())
	b := rec(t, 0, with(baseCtx(), "blaze.gsid", "9999"))
	if find(ComparePair(a, b), Excluded, "blaze.gsid") == nil {
		t.Error("unused field should be EXCLUDED")
	}

	h := a
	body := h.BodyBytes()
	body[0] ^= 1
	h.Body = hexOf(body)
	if find(ComparePair(a, h), Hidden, "-") == nil {
		t.Error("body change without context change should be HIDDEN")
	}
}

func TestAnalyzeVerdicts(t *testing.T) {
	var recs []Record
	for _, c := range []byte{0, 3, 6} {
		recs = append(recs,
			rec(t, c, baseCtx()),
			rec(t, c, with(baseCtx(), "blaze.uuid", "f47ac10b-58cc-4372-a567-0e02b2c3d479")),
			rec(t, c, with(baseCtx(), "blaze.seed", "7")),
			rec(t, c, with(baseCtx(), "blaze.gsid", "9999")),
		)
	}
	r := Analyze(recs)
	if s := r.Verdicts["blaze.uuid"].Summary(); !strings.Contains(s, "IN PLAINTEXT") || !strings.Contains(s, "offset=4 enc=guid-raw") {
		t.Errorf("uuid verdict = %q", s)
	}
	if s := r.Verdicts["blaze.seed"].Summary(); !strings.HasPrefix(s, "INPUT") {
		t.Errorf("seed verdict = %q", s)
	}
	if s := r.Verdicts["blaze.gsid"].Summary(); !strings.HasPrefix(s, "not an input") {
		t.Errorf("gsid verdict = %q", s)
	}
	if s := r.Verdicts["server.port"].Summary(); !strings.Contains(s, "never varied") {
		t.Errorf("port verdict = %q", s)
	}
}

func TestStorePersistsAndDedupes(t *testing.T) {
	path := t.TempDir() + "/hs.jsonl"
	st, _ := OpenStore(path)
	r := rec(t, 0, baseCtx())
	if _, isNew, err := st.Add(r); !isNew || err != nil {
		t.Fatalf("first Add: new=%v err=%v", isNew, err)
	}
	if _, isNew, _ := st.Add(r); isNew {
		t.Error("duplicate stored twice")
	}
	st2, err := OpenStore(path)
	if err != nil || len(st2.Records()) != 1 {
		t.Fatalf("reload: %d records, err=%v", len(st2.Records()), err)
	}
}

func hexOf(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, v := range b {
		out = append(out, digits[v>>4], digits[v&15])
	}
	return string(out)
}
