package handshake

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	RequestLen  = 24
	RequestFlag = 0x80
	BodyLen     = RequestLen - 2
)

func Parse(pkt []byte) (counter, flag byte, body []byte, ok bool) {
	if len(pkt) != RequestLen || pkt[1] != RequestFlag {
		return 0, 0, nil, false
	}
	return pkt[0], pkt[1], pkt[2:], true
}

type Record struct {
	Time    time.Time         `json:"time"`
	From    string            `json:"from"`
	Counter byte              `json:"counter"`
	Flag    byte              `json:"flag"`
	Body    string            `json:"body"`
	Ctx     map[string]string `json:"ctx"`
}

func NewRecord(from string, pkt []byte, ctx map[string]string) (Record, bool) {
	c, f, body, ok := Parse(pkt)
	if !ok {
		return Record{}, false
	}
	cp := make(map[string]string, len(ctx))
	for k, v := range ctx {
		cp[k] = v
	}
	return Record{Time: time.Now(), From: from, Counter: c, Flag: f, Body: hex.EncodeToString(body), Ctx: cp}, true
}

func (r Record) BodyBytes() []byte {
	b, _ := hex.DecodeString(r.Body)
	return b
}

func (r Record) ctxKey() string {
	keys := make([]string, 0, len(r.Ctx))
	for k := range r.Ctx {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k + "=" + r.Ctx[k] + ";")
	}
	return sb.String()
}

type Store struct {
	path string
	mu   sync.Mutex
	recs []Record
	seen map[string]bool
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, seen: map[string]bool{}}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) != nil || len(r.BodyBytes()) == 0 {
			continue
		}
		if s.markSeen(r) {
			s.recs = append(s.recs, r)
		}
	}
	return s, sc.Err()
}

func (s *Store) markSeen(r Record) bool {
	k := fmt.Sprintf("%d|%s|%s", r.Counter, r.Body, r.ctxKey())
	if s.seen[k] {
		return false
	}
	s.seen[k] = true
	return true
}

func (s *Store) Add(r Record) (prior []Record, isNew bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.markSeen(r) {
		return nil, false, nil
	}
	prior = append([]Record(nil), s.recs...)
	s.recs = append(s.recs, r)
	if s.path == "" {
		return prior, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return prior, true, err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return prior, true, err
	}
	defer f.Close()
	line, _ := json.Marshal(r)
	_, err = f.Write(append(line, '\n'))
	return prior, true, err
}

func (s *Store) Records() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Record(nil), s.recs...)
}
