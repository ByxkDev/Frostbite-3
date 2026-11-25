package ebx

import (
	"fmt"
	"slices"
	"strings"
)

type Instance struct {
	GUID    GUID           `json:"guid"`
	Type    string         `json:"type"`
	Primary bool           `json:"primary,omitempty"`
	Fields  map[string]any `json:"fields,omitempty"`
}

func NewInstance(typ string, guid GUID) *Instance {
	return &Instance{GUID: guid, Type: typ, Fields: map[string]any{}}
}

func (in *Instance) Set(name string, v any) *Instance {
	if in.Fields == nil {
		in.Fields = map[string]any{}
	}
	in.Fields[name] = v
	return in
}

func (in *Instance) Float(name string) (float64, bool) {
	switch v := in.Fields[name].(type) {
	case float64:
		return v, true
	case int64:
		return float64(v), true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
}

func (in *Instance) StringField(name string) (string, bool) {
	s, ok := in.Fields[name].(string)
	return s, ok
}

func (in *Instance) RefField(name string) (Ref, bool) {
	r, ok := in.Fields[name].(Ref)
	return r, ok
}

type Partition struct {
	Path      string      `json:"path"`
	GUID      GUID        `json:"guid"`
	Instances []*Instance `json:"instances"`
}

func NewPartition(path string, guid GUID) *Partition {
	if guid.IsZero() {
		guid = DeriveGUID("partition/" + path)
	}
	return &Partition{Path: path, GUID: guid}
}

func (p *Partition) Add(in *Instance, primary bool) *Instance {
	if primary {
		for _, existing := range p.Instances {
			existing.Primary = false
		}
		in.Primary = true
	} else if len(p.Instances) == 0 {
		in.Primary = true
	}
	p.Instances = append(p.Instances, in)
	return in
}

func (p *Partition) Primary() *Instance {
	for _, in := range p.Instances {
		if in.Primary {
			return in
		}
	}
	return nil
}

func (p *Partition) FindByGUID(g GUID) *Instance {
	for _, in := range p.Instances {
		if in.GUID == g {
			return in
		}
	}
	return nil
}

func (p *Partition) FindByType(typ string) *Instance {
	for _, in := range p.Instances {
		if strings.EqualFold(in.Type, typ) {
			return in
		}
	}
	return nil
}

func (p *Partition) Ref(in *Instance) Ref {
	return Ref{Partition: p.GUID, Instance: in.GUID}
}

type DB struct {
	byPath map[string]*Partition
	byGUID map[GUID]*Partition
}

func NewDB() *DB {
	return &DB{byPath: map[string]*Partition{}, byGUID: map[GUID]*Partition{}}
}

func (db *DB) Add(p *Partition) {
	db.byPath[strings.ToLower(p.Path)] = p
	db.byGUID[p.GUID] = p
}

func (db *DB) Partition(path string) (*Partition, bool) {
	p, ok := db.byPath[strings.ToLower(strings.TrimSpace(path))]
	return p, ok
}

func (db *DB) ByGUID(g GUID) (*Partition, bool) {
	p, ok := db.byGUID[g]
	return p, ok
}

func (db *DB) Resolve(holder *Partition, r Ref) (*Instance, bool) {
	if r.IsNull() {
		return nil, false
	}
	target := holder
	if r.IsExternal() {
		p, ok := db.ByGUID(r.Partition)
		if !ok {
			return nil, false
		}
		target = p
	}
	if target == nil {
		return nil, false
	}
	in := target.FindByGUID(r.Instance)
	return in, in != nil
}

func (db *DB) Paths() []string {
	out := make([]string, 0, len(db.byPath))
	for _, p := range db.byPath {
		out = append(out, p.Path)
	}
	slices.Sort(out)
	return out
}

func (db *DB) Len() int { return len(db.byGUID) }

func (db *DB) String() string {
	return fmt.Sprintf("ebx.DB(%d partitions)", db.Len())
}
