package server

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"bf4/logger"
)

var (
	HSProbeFile    = "data/hsprobe.json"
	HSProbeCurrent = "data/hsprobe.current.json"
)

type hsProbeConfig struct {
	SEED *uint32  `json:"seed,omitempty"`
	GSID *uint64  `json:"gsid,omitempty"`
	HSES *uint32  `json:"hses,omitempty"`
	GPVH *uint64  `json:"gpvh,omitempty"`
	UUID *string  `json:"uuid,omitempty"`
	PGID *string  `json:"pgid,omitempty"`
	XNNC *string  `json:"xnnc,omitempty"`
	XSES *string  `json:"xses,omitempty"`
	PGSR *string  `json:"pgsr,omitempty"`
	Vary []string `json:"vary,omitempty"`
}

type hsVaried struct {
	at     time.Time
	values map[string]string
}

var (
	hsProbeMu sync.Mutex
	hsVaryBy  = map[uint32]*hsVaried{}
	hsCurrent = map[string]map[string]string{}
)

const hsVaryWindow = 5 * time.Second

func loadHSProbe() *hsProbeConfig {
	b, err := os.ReadFile(HSProbeFile)
	if err != nil {
		return nil
	}
	var c hsProbeConfig
	if err := json.Unmarshal(b, &c); err != nil {
		logger.Warn("HSPROBE: %s: %v (ignored)", HSProbeFile, err)
		return nil
	}
	return &c
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func randUUID() string {
	b := randBytes(16)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func randomValue(field string) (string, bool) {
	switch field {
	case "seed", "hses":
		return strconv.FormatUint(uint64(binary.BigEndian.Uint32(randBytes(4))), 10), true
	case "gsid", "gpvh":
		return strconv.FormatUint(binary.BigEndian.Uint64(randBytes(8))>>1, 10), true
	case "uuid", "pgid":
		return randUUID(), true
	case "xnnc", "xses", "pgsr":
		return "hex:" + hex.EncodeToString(randBytes(16)), true
	}
	return "", false
}

func hexField(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(s), "hex:"))
}

func applyHSProbe(g *Game, joiner *Player, d *BF4GameData) {
	cfg := loadHSProbe()

	hsProbeMu.Lock()
	defer hsProbeMu.Unlock()

	set := map[string]string{}
	if cfg != nil {
		if cfg.SEED != nil {
			set["seed"] = strconv.FormatUint(uint64(*cfg.SEED), 10)
		}
		if cfg.GSID != nil {
			set["gsid"] = strconv.FormatUint(*cfg.GSID, 10)
		}
		if cfg.HSES != nil {
			set["hses"] = strconv.FormatUint(uint64(*cfg.HSES), 10)
		}
		if cfg.GPVH != nil {
			set["gpvh"] = strconv.FormatUint(*cfg.GPVH, 10)
		}
		for k, p := range map[string]*string{"uuid": cfg.UUID, "pgid": cfg.PGID, "xnnc": cfg.XNNC, "xses": cfg.XSES, "pgsr": cfg.PGSR} {
			if p != nil {
				set[k] = *p
			}
		}
		if len(cfg.Vary) > 0 {
			v := hsVaryBy[g.ID]
			if v == nil || joiner != nil && time.Since(v.at) > hsVaryWindow {
				v = &hsVaried{at: time.Now(), values: map[string]string{}}
				for _, f := range cfg.Vary {
					f = strings.ToLower(strings.TrimSpace(f))
					if val, ok := randomValue(f); ok {
						v.values[f] = val
					} else {
						logger.Warn("HSPROBE: can't vary unknown field %q", f)
					}
				}
				hsVaryBy[g.ID] = v
				logger.Info("HSPROBE: game %d new varied values %v", g.ID, v.values)
			}
			for k, val := range v.values {
				set[k] = val
			}
		}
	}

	for k, val := range set {
		var err error
		var u uint64
		switch k {
		case "seed":
			u, err = strconv.ParseUint(val, 10, 32)
			d.SEED = uint32(u)
		case "hses":
			u, err = strconv.ParseUint(val, 10, 32)
			d.HSES = uint32(u)
		case "gsid":
			d.GSID, err = strconv.ParseUint(val, 10, 64)
		case "gpvh":
			d.GPVH, err = strconv.ParseUint(val, 10, 64)
		case "uuid":
			d.UUID = val
		case "pgid":
			d.PGID = val
		case "xnnc":
			d.XNNC, err = hexField(val)
		case "xses":
			d.XSES, err = hexField(val)
		case "pgsr":
			d.PGSR, err = hexField(val)
		}
		if err != nil {
			logger.Warn("HSPROBE: bad %s value %q: %v", k, val, err)
		}
	}

	cur := map[string]string{
		"gid":  strconv.FormatUint(uint64(d.GID), 10),
		"seed": strconv.FormatUint(uint64(d.SEED), 10),
		"gsid": strconv.FormatUint(d.GSID, 10),
		"hses": strconv.FormatUint(uint64(d.HSES), 10),
		"gpvh": strconv.FormatUint(d.GPVH, 10),
		"uuid": d.UUID,
		"pgid": d.PGID,
		"xnnc": "hex:" + hex.EncodeToString(d.XNNC),
		"xses": "hex:" + hex.EncodeToString(d.XSES),
		"pgsr": "hex:" + hex.EncodeToString(d.PGSR),
	}
	key := strconv.FormatUint(uint64(g.ID), 10)
	if old, ok := hsCurrent[key]; ok && mapsEqual(old, cur) {
		return
	}
	hsCurrent[key] = cur
	if out, err := json.MarshalIndent(hsCurrent, "", "  "); err == nil {
		if err := os.MkdirAll(dirOf(HSProbeCurrent), 0o755); err == nil {
			_ = os.WriteFile(HSProbeCurrent, out, 0o644)
		}
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
