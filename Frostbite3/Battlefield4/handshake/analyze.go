package handshake

import (
	"encoding/hex"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"

	"bf4/ebx"
)

type Kind int

const (
	Excluded Kind = iota

	Input

	Plaintext

	Ambiguous

	Hidden
)

func (k Kind) String() string {
	return [...]string{"EXCLUDED", "INPUT", "PLAINTEXT", "AMBIGUOUS", "HIDDEN"}[k]
}

type Finding struct {
	Kind     Kind
	Field    string
	Counter  byte
	Offset   int
	Encoding string
	Clean    bool
	Detail   string
}

func (f Finding) String() string {
	switch f.Kind {
	case Plaintext:
		clean := "other bytes also changed"
		if f.Clean {
			clean = "no other byte changed"
		}
		return fmt.Sprintf("counter=%d %s %q at body offset %d (packet offset %d) as %s; %s",
			f.Counter, f.Kind, f.Field, f.Offset, f.Offset+2, f.Encoding, clean)
	default:
		return fmt.Sprintf("counter=%d %s %s %s", f.Counter, f.Kind, f.Field, f.Detail)
	}
}

type encoding struct {
	name string
	b    []byte
}

func encodings(v string) []encoding {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	var out []encoding
	if strings.HasPrefix(v, "hex:") {
		if b, err := hex.DecodeString(v[4:]); err == nil && len(b) > 0 {
			out = append(out, encoding{"bytes", b})
		}
		return out
	}
	if u, err := strconv.ParseUint(v, 10, 64); err == nil {
		for _, w := range []int{2, 4, 8} {
			if w < 8 && u >= 1<<(8*uint(w)) {
				continue
			}
			be, le := make([]byte, w), make([]byte, w)
			for i := range w {
				be[w-1-i] = byte(u >> (8 * uint(i)))
				le[i] = byte(u >> (8 * uint(i)))
			}
			out = append(out, encoding{fmt.Sprintf("u%dbe", w*8), be}, encoding{fmt.Sprintf("u%dle", w*8), le})
		}
	}
	if ip := net.ParseIP(v).To4(); ip != nil && strings.Count(v, ".") == 3 {
		out = append(out, encoding{"ip4be", []byte(ip)}, encoding{"ip4le", []byte{ip[3], ip[2], ip[1], ip[0]}})
	}
	if g, err := ebx.ParseGUID(v); err == nil {
		raw := g[:]
		ms := []byte{raw[3], raw[2], raw[1], raw[0], raw[5], raw[4], raw[7], raw[6]}
		ms = append(ms, raw[8:]...)
		out = append(out, encoding{"guid-raw", append([]byte(nil), raw...)}, encoding{"guid-ms", ms})
	}
	out = append(out, encoding{"ascii", []byte(v)})
	return out
}

const minOverlap = 4

func plaintextMatches(field, va, vb string, d []byte, counter byte) []Finding {
	var out []Finding
	ea, eb := encodings(va), encodings(vb)
	for _, a := range ea {
		for _, b := range eb {
			if a.name != b.name || len(a.b) != len(b.b) {
				continue
			}
			x := make([]byte, len(a.b))
			nz := 0
			for i := range x {
				x[i] = a.b[i] ^ b.b[i]
				if x[i] != 0 {
					nz++
				}
			}
			if nz == 0 {
				continue
			}
			L := len(x)
			need := minOverlap
			if L < need {
				need = L
			}
			for off := -(L - need); off <= len(d)-need; off++ {
				lo, hi := max(off, 0), min(off+L, len(d))
				if hi-lo < need {
					continue
				}
				match, nzHit := true, 0
				for i := lo; i < hi; i++ {
					if d[i] != x[i-off] {
						match = false
						break
					}
					if x[i-off] != 0 {
						nzHit++
					}
				}
				if !match || nzHit < 2 && !(L <= 2 && nzHit >= 1) {
					continue
				}
				clean := true
				for i := range d {
					if (i < lo || i >= hi) && d[i] != 0 {
						clean = false
						break
					}
				}
				out = append(out, Finding{Kind: Plaintext, Field: field, Counter: counter,
					Offset: off, Encoding: a.name, Clean: clean})
			}
		}
	}
	return out
}

func diffShape(d []byte) (changed int, first, last int) {
	first, last = -1, -1
	for i, v := range d {
		if v != 0 {
			changed++
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	return
}

func shapeDetail(d []byte) string {
	n, first, last := diffShape(d)
	if n*3 >= len(d)*2 {
		return fmt.Sprintf("(%d/%d body bytes changed -> diffused: key/IV material or block cipher/MAC)", n, len(d))
	}
	return fmt.Sprintf("(only %d/%d body bytes changed, at %d..%d -> localized: stream cipher, field lives there)", n, len(d), first, last)
}

func ComparePair(a, b Record) []Finding {
	if a.Counter != b.Counter {
		return nil
	}
	ba, bb := a.BodyBytes(), b.BodyBytes()
	if len(ba) != len(bb) || len(ba) == 0 {
		return nil
	}
	d := make([]byte, len(ba))
	same := true
	for i := range d {
		d[i] = ba[i] ^ bb[i]
		if d[i] != 0 {
			same = false
		}
	}

	var diff []string
	keys := map[string]bool{}
	for k := range a.Ctx {
		keys[k] = true
	}
	for k := range b.Ctx {
		keys[k] = true
	}
	for k := range keys {
		if a.Ctx[k] != b.Ctx[k] {
			diff = append(diff, k)
		}
	}
	slices.Sort(diff)

	var out []Finding
	switch {
	case same:
		for _, k := range diff {
			out = append(out, Finding{Kind: Excluded, Field: k, Counter: a.Counter,
				Detail: fmt.Sprintf("(%q -> %q, body identical)", a.Ctx[k], b.Ctx[k])})
		}
	case len(diff) == 0:
		out = append(out, Finding{Kind: Hidden, Field: "-", Counter: a.Counter,
			Detail: "same recorded context but different body " + shapeDetail(d)})
	default:
		var pt []Finding
		for _, k := range diff {
			pt = append(pt, plaintextMatches(k, a.Ctx[k], b.Ctx[k], d, a.Counter)...)
		}
		out = append(out, pt...)
		if len(diff) == 1 {
			out = append(out, Finding{Kind: Input, Field: diff[0], Counter: a.Counter,
				Detail: fmt.Sprintf("(%q -> %q) %s", a.Ctx[diff[0]], b.Ctx[diff[0]], shapeDetail(d))})
		} else if len(pt) == 0 {
			out = append(out, Finding{Kind: Ambiguous, Field: strings.Join(diff, ","), Counter: a.Counter,
				Detail: shapeDetail(d) + " -- vary one field at a time"})
		}
	}
	return out
}

type Verdict struct {
	Field     string
	Excluded  int
	Input     int
	Plaintext map[string]int
	Varied    bool
}

func (v *Verdict) Summary() string {
	if len(v.Plaintext) > 0 {
		n := 0
		for _, c := range v.Plaintext {
			n = max(n, c)
		}
		var best []string
		for k, c := range v.Plaintext {
			if c == n {
				best = append(best, k)
			}
		}
		slices.Sort(best)
		tie := ""
		if len(best) > 1 {
			tie = " [ambiguous between these; vary with random values to split]"
		}
		return fmt.Sprintf("IN PLAINTEXT  %s (%d hits)%s -- stream cipher, this field is readable",
			strings.Join(best, " | "), n, tie)
	}
	switch {
	case v.Input > 0 && v.Excluded > 0:
		return fmt.Sprintf("CONFLICT      input %dx, excluded %dx -- check captures", v.Input, v.Excluded)
	case v.Input > 0:
		return fmt.Sprintf("INPUT         changed the body %dx (key/IV material or diffused)", v.Input)
	case v.Excluded > 0:
		return fmt.Sprintf("not an input  (varied %dx, body never changed)", v.Excluded)
	case v.Varied:
		return "untested      only ever varied together with other fields"
	default:
		return "untested      never varied -- change it between joins"
	}
}

type Report struct {
	Records  int
	Pairs    int
	Hidden   int
	Verdicts map[string]*Verdict
	Findings []Finding
}

const maxAnalyze = 4000

func Analyze(recs []Record) *Report {
	if len(recs) > maxAnalyze {
		recs = recs[len(recs)-maxAnalyze:]
	}
	r := &Report{Records: len(recs), Verdicts: map[string]*Verdict{}}
	verdict := func(f string) *Verdict {
		v := r.Verdicts[f]
		if v == nil {
			v = &Verdict{Field: f, Plaintext: map[string]int{}}
			r.Verdicts[f] = v
		}
		return v
	}
	for _, rec := range recs {
		for k := range rec.Ctx {
			verdict(k)
		}
	}
	byCounter := map[byte][]Record{}
	for _, rec := range recs {
		byCounter[rec.Counter] = append(byCounter[rec.Counter], rec)
	}
	for _, group := range byCounter {
		for i := range len(group) {
			for j := i + 1; j < len(group); j++ {
				r.Pairs++
				for _, f := range ComparePair(group[i], group[j]) {
					r.Findings = append(r.Findings, f)
					switch f.Kind {
					case Excluded:
						v := verdict(f.Field)
						v.Excluded++
						v.Varied = true
					case Input:
						v := verdict(f.Field)
						v.Input++
						v.Varied = true
					case Plaintext:
						v := verdict(f.Field)
						v.Plaintext[fmt.Sprintf("offset=%d enc=%s", f.Offset, f.Encoding)]++
						v.Varied = true
					case Ambiguous:
						for k := range strings.SplitSeq(f.Field, ",") {
							verdict(k).Varied = true
						}
					case Hidden:
						r.Hidden++
					}
				}
			}
		}
	}
	return r
}

func (r *Report) Lines() []string {
	out := []string{fmt.Sprintf("%d records, %d same-counter pairs compared", r.Records, r.Pairs)}
	fields := make([]string, 0, len(r.Verdicts))
	for f := range r.Verdicts {
		fields = append(fields, f)
	}
	slices.Sort(fields)
	var untested []string
	for _, f := range fields {
		v := r.Verdicts[f]
		out = append(out, fmt.Sprintf("  %-14s %s", f, v.Summary()))
		if v.Excluded == 0 && v.Input == 0 && len(v.Plaintext) == 0 {
			untested = append(untested, f)
		}
	}
	if r.Hidden > 0 {
		out = append(out, fmt.Sprintf("  !! %d pairs changed with NO recorded difference: an unrecorded input exists (client-side value, clock, RNG)", r.Hidden))
	}
	if len(untested) > 0 {
		out = append(out, "  next: vary one of "+strings.Join(untested, ", "))
	}
	return out
}
