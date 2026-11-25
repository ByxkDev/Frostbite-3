package ebx

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const DefaultRegistryDir = "data/ebx/registry"

func LoadNetworkRegistryDir(dir string) (*NetworkRegistry, error) {
	r := NewNetworkRegistry()
	entries, err := filepath.Glob(filepath.Join(dir, "*_networkregistry_Ps3.txt"))
	if err != nil {
		return nil, err
	}
	for _, path := range entries {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		p, err := ParseNetworkRegistry(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("ebx: parsing %s: %w", path, err)
		}
		r.AddPartition(p)
	}
	return r, nil
}

type NetworkRegistry struct {
	parts map[string]*RegistryPartition
}

type RegistryObject struct {
	Index int
	Path  string
	GUID  GUID
}

type RegistryPartition struct {
	Name    string
	GUID    GUID
	Objects []RegistryObject
	byGUID  map[GUID]int
}

func NewNetworkRegistry() *NetworkRegistry {
	return &NetworkRegistry{parts: map[string]*RegistryPartition{}}
}

func (r *NetworkRegistry) Partition(name string) *RegistryPartition {
	return r.parts[strings.ToLower(name)]
}

func (r *NetworkRegistry) PartitionNames() []string {
	out := make([]string, 0, len(r.parts))
	for n := range r.parts {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

func (r *NetworkRegistry) Total(partition string) int {
	p := r.Partition(partition)
	if p == nil {
		return 0
	}
	return len(p.Objects)
}

func (r *NetworkRegistry) DataRefIndex(partition string, g GUID) (index, total int, ok bool) {
	p := r.Partition(partition)
	if p == nil {
		return 0, 0, false
	}
	if i, found := p.byGUID[g]; found {
		return i, len(p.Objects), true
	}
	return 0, len(p.Objects), false
}

func (r *NetworkRegistry) OwnerRefIndex(partition string, g GUID) (index uint32, ok bool) {
	if g.IsZero() {
		return 0, true
	}
	p := r.Partition(partition)
	if p == nil {
		return 0, false
	}
	if i, found := p.byGUID[g]; found {
		return uint32(i + 1), true
	}
	return 0, false
}

func (p *RegistryPartition) Find(sub string) (RegistryObject, bool) {
	sub = strings.ToLower(sub)
	for _, o := range p.Objects {
		if strings.Contains(strings.ToLower(o.Path), sub) {
			return o, true
		}
	}
	return RegistryObject{}, false
}

func DataRefPartition(subLevelID int) int { return subLevelID - 3 }

var reMember = regexp.MustCompile(`^member\((\d+)\)\s+(.*\S)\s*$`)

func ParseNetworkRegistry(rd io.Reader) (*RegistryPartition, error) {
	p := &RegistryPartition{byGUID: map[GUID]int{}}
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<20)
	first := true
	for sc.Scan() {
		line := sc.Text()
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		if first {
			if g, err := ParseGUID(s); err == nil {
				p.GUID = g
			}
			first = false
			continue
		}
		if p.Name == "" && strings.HasPrefix(s, "Name ") {
			p.Name = strings.TrimSpace(s[len("Name "):])
			continue
		}
		if m := reMember.FindStringSubmatch(s); m != nil {
			ref := m[2]

			if len(ref) < 37 || ref[len(ref)-37] != '/' {
				continue
			}
			g, err := ParseGUID(ref[len(ref)-36:])
			if err != nil {
				continue
			}
			idx := len(p.Objects)
			p.Objects = append(p.Objects, RegistryObject{Index: idx, Path: ref[:len(ref)-37], GUID: g})
			if _, dup := p.byGUID[g]; !dup {
				p.byGUID[g] = idx
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if p.Name == "" {
		return nil, fmt.Errorf("ebx: no Name line in network registry dump")
	}
	return p, nil
}

func (r *NetworkRegistry) AddPartition(p *RegistryPartition) {
	r.parts[strings.ToLower(p.Name)] = p
}

func (r *NetworkRegistry) String() string {
	parts := make([]string, 0, len(r.parts))
	for _, n := range r.PartitionNames() {
		parts = append(parts, fmt.Sprintf("%s:%d", n, len(r.parts[n].Objects)))
	}
	if len(parts) == 0 {
		return "ebx.NetworkRegistry(empty)"
	}
	return "ebx.NetworkRegistry(" + strings.Join(parts, " ") + ")"
}
