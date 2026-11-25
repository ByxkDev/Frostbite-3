package ebx

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

type GUID [16]byte

var Zero GUID

func (g GUID) IsZero() bool { return g == Zero }

func (g GUID) String() string {
	h := hex.EncodeToString(g[:])
	return strings.Join([]string{h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]}, "-")
}

func (g GUID) MarshalText() ([]byte, error) { return []byte(g.String()), nil }

func (g *GUID) UnmarshalText(b []byte) error {
	v, err := ParseGUID(string(b))
	if err != nil {
		return err
	}
	*g = v
	return nil
}

func ParseGUID(s string) (GUID, error) {
	clean := strings.ReplaceAll(strings.TrimSpace(s), "-", "")
	if len(clean) != 32 {
		return Zero, fmt.Errorf("ebx: bad GUID %q: want 32 hex digits, got %d", s, len(clean))
	}
	raw, err := hex.DecodeString(clean)
	if err != nil {
		return Zero, fmt.Errorf("ebx: bad GUID %q: %w", s, err)
	}
	var g GUID
	copy(g[:], raw)
	return g, nil
}

func MustGUID(s string) GUID {
	g, err := ParseGUID(s)
	if err != nil {
		panic(err)
	}
	return g
}

func DeriveGUID(seed string) GUID {
	sum := sha1.Sum([]byte("frostbite3/ebx:" + seed))
	var g GUID
	copy(g[:], sum[:16])
	return g
}

type Ref struct {
	Partition GUID `json:"partition,omitempty"`
	Instance  GUID `json:"instance"`
}

func (r Ref) IsExternal() bool { return !r.Partition.IsZero() }

func (r Ref) IsNull() bool { return r.Instance.IsZero() }

func (r Ref) String() string {
	if r.IsNull() {
		return "<null ref>"
	}
	if r.IsExternal() {
		return r.Partition.String() + "/" + r.Instance.String()
	}
	return r.Instance.String()
}
