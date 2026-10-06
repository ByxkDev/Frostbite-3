package components

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Persona struct {
	ID     uint64
	Name   string
	Online bool
}

func PersonaIDFor(name string) uint64 { return personaIDFromOnlineID(name) }

func userDirName(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return '_'
	}, name)
}

func KnownPersonas() []Persona {
	byID := map[uint64]*Persona{}
	if entries, err := os.ReadDir(UsersDir); err == nil {
		for _, e := range entries {
			if e.IsDir() && e.Name() != "unknown" {
				id := personaIDFromOnlineID(e.Name())
				byID[id] = &Persona{ID: id, Name: e.Name()}
			}
		}
	}
	for _, s := range OnlinePlayers() {
		if s.OnlineID == "" {
			continue
		}
		id := s.PersonaID
		if id == 0 {
			id = personaIDFromOnlineID(s.OnlineID)
		}
		if p, ok := byID[id]; ok {
			p.Name, p.Online = s.OnlineID, true
		} else {
			byID[id] = &Persona{ID: id, Name: s.OnlineID, Online: true}
		}
	}
	out := make([]Persona, 0, len(byID))
	for _, p := range byID {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func FindPersona(id uint64) (Persona, bool) {
	for _, p := range KnownPersonas() {
		if p.ID == id {
			return p, true
		}
	}
	return Persona{}, false
}

func FindPersonaByName(name string) (Persona, bool) {
	for _, p := range KnownPersonas() {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Persona{}, false
}

func PersonaStats(name string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(filepath.Join(UsersDir, userDirName(name), "stats.txt"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}
