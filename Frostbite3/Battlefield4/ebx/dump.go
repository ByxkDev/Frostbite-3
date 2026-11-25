package ebx

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

const DefaultDumpDir = "data/ebx/dump"

type DumpNode struct {
	Name     string
	Type     string
	Value    string
	Children []*DumpNode
}

type DumpInstance struct {
	Type      string
	ID        string
	Partition *DumpPartition
	Bases     []string
	Root      *DumpNode
}

type DumpPartition struct {
	GUID      string
	Name      string
	File      string
	Source    string
	Instances []*DumpInstance
	byID      map[string]*DumpInstance

	path    string
	once    sync.Once
	loadErr error
}

type Store struct {
	parts    map[string]*DumpPartition
	byFile   map[string]*DumpPartition
	byGUID   map[string]*DumpInstance
	byType   map[string][]*DumpInstance
	nInst    int
	nFields  int
	replaced int
}

func NewStore() *Store {
	return &Store{
		parts:  map[string]*DumpPartition{},
		byFile: map[string]*DumpPartition{},
		byGUID: map[string]*DumpInstance{},
		byType: map[string][]*DumpInstance{},
	}
}

const DefaultSbTocDir = "data/ebx/sbtoc"

func LoadDumpDir(dir string) (*Store, error) { return LoadDumpDirs(dir) }

func LoadDumpDirs(dirs ...string) (*Store, error) {
	s := NewStore()
	for _, d := range dirs {
		if err := s.LoadDir(d); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) LoadDir(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".txt") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	slices.Sort(files)
	for _, p := range files {
		part, err := indexDump(p)
		if err != nil {
			return fmt.Errorf("ebx: indexing %s: %w", p, err)
		}
		rel, _ := filepath.Rel(dir, p)
		part.File = filepath.ToSlash(rel)
		part.Source = dir
		s.Add(part)
	}
	return nil
}

func indexDump(path string) (*DumpPartition, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	part := &DumpPartition{byID: map[string]*DumpInstance{}, path: path}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<22)
	first := true
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '\r' {
			continue
		}
		if line[0] != '\t' {
			body := strings.TrimSpace(line)
			if body == "" {
				continue
			}
			if first {
				first = false
				if !strings.Contains(body, " ") {
					part.GUID = body
					continue
				}
			}
			typ, id, _ := strings.Cut(body, " ")
			in := &DumpInstance{Type: typ, ID: strings.TrimSpace(id), Partition: part}
			part.Instances = append(part.Instances, in)
			part.byID[strings.ToLower(in.ID)] = in
			continue
		}
		first = false
		if part.Name == "" && len(part.Instances) == 1 {
			body := strings.TrimSpace(line)
			if v, ok := strings.CutPrefix(body, "Name "); ok {
				part.Name = strings.TrimSpace(v)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if part.Name == "" && len(part.Instances) > 0 {
		part.Name = part.Instances[0].ID
	}
	return part, nil
}

func (p *DumpPartition) load() {
	if p.path == "" {
		return
	}
	p.once.Do(func() {
		f, err := os.Open(p.path)
		if err != nil {
			p.loadErr = err
			return
		}
		full, err := ParseDump(f)
		f.Close()
		if err != nil {
			p.loadErr = err
			return
		}
		for i, in := range p.Instances {
			if i < len(full.Instances) && full.Instances[i].ID == in.ID {
				in.Root, in.Bases = full.Instances[i].Root, full.Instances[i].Bases
			}
		}
	})
}

func (p *DumpPartition) LoadErr() error { p.load(); return p.loadErr }

func (in *DumpInstance) tree() *DumpNode {
	in.Partition.load()
	if in.Root == nil {
		in.Root = &DumpNode{Name: in.Type, Type: in.Type}
	}
	return in.Root
}

func ParseDump(rd io.Reader) (*DumpPartition, error) {
	part := &DumpPartition{byID: map[string]*DumpInstance{}}
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<22)
	type frame struct {
		depth int
		node  *DumpNode
	}
	var inst *DumpInstance
	var stack []frame
	first := true
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		depth := 0
		for depth < len(line) && line[depth] == '\t' {
			depth++
		}
		body := strings.TrimSpace(line[depth:])
		if first {
			first = false
			if depth == 0 && !strings.Contains(body, " ") {
				part.GUID = body
				continue
			}
		}
		if depth == 0 {
			typ, id, _ := strings.Cut(body, " ")
			inst = &DumpInstance{Type: typ, ID: strings.TrimSpace(id), Partition: part, Root: &DumpNode{Name: typ, Type: typ}}
			part.Instances = append(part.Instances, inst)
			part.byID[strings.ToLower(inst.ID)] = inst
			stack = []frame{{0, inst.Root}}
			continue
		}
		if inst == nil {
			continue
		}
		n := parseNodeLine(body)
		if n.Name == "$" {
			inst.Bases = append(inst.Bases, n.Type)
		}
		for len(stack) > 1 && stack[len(stack)-1].depth >= depth {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1].node
		parent.Children = append(parent.Children, n)
		stack = append(stack, frame{depth, n})
		if part.Name == "" && n.Name == "Name" && len(part.Instances) == 1 {
			part.Name = n.Value
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if part.Name == "" && len(part.Instances) > 0 {
		part.Name = part.Instances[0].ID
	}
	return part, nil
}

func parseNodeLine(body string) *DumpNode {
	if strings.HasPrefix(body, "$::") {
		return &DumpNode{Name: "$", Type: body[3:]}
	}
	name, rest, hasVal := strings.Cut(body, " ")
	n := &DumpNode{Name: name}
	if i := strings.Index(name, "::"); i >= 0 {
		n.Name, n.Type = name[:i], name[i+2:]
	}
	if hasVal {
		n.Value = strings.TrimSpace(rest)
	}
	return n
}

func (s *Store) Add(p *DumpPartition) {
	if old := s.parts[strings.ToLower(p.Name)]; old != nil {
		s.remove(old)
		s.replaced++
	}
	s.parts[strings.ToLower(p.Name)] = p
	if p.File != "" {
		s.byFile[strings.ToLower(p.File)] = p
	}
	for _, in := range p.Instances {
		s.nInst++
		s.nFields += countNodes(in.Root) - 1
		if _, err := ParseGUID(in.ID); err == nil {
			s.byGUID[strings.ToLower(in.ID)] = in
		}
		s.byType[in.Type] = append(s.byType[in.Type], in)
	}
}

func (s *Store) remove(p *DumpPartition) {
	if p.File != "" && s.byFile[strings.ToLower(p.File)] == p {
		delete(s.byFile, strings.ToLower(p.File))
	}
	for _, in := range p.Instances {
		s.nInst--
		s.nFields -= countNodes(in.Root) - 1
		if s.byGUID[strings.ToLower(in.ID)] == in {
			delete(s.byGUID, strings.ToLower(in.ID))
		}
		l := s.byType[in.Type]
		for i, x := range l {
			if x == in {
				s.byType[in.Type] = append(l[:i:i], l[i+1:]...)
				break
			}
		}
		if len(s.byType[in.Type]) == 0 {
			delete(s.byType, in.Type)
		}
	}
}

func (s *Store) Replaced() int { return s.replaced }

func (s *Store) CountFrom(dir string) int {
	n := 0
	for _, p := range s.parts {
		if p.Source == dir {
			n++
		}
	}
	return n
}

func countNodes(n *DumpNode) int {
	if n == nil {
		return 1
	}
	c := 1
	for _, k := range n.Children {
		c += countNodes(k)
	}
	return c
}

func (s *Store) Partition(name string) *DumpPartition { return s.parts[strings.ToLower(name)] }

func (s *Store) PartitionFile(rel string) *DumpPartition {
	return s.byFile[strings.ToLower(filepath.ToSlash(rel))]
}

func (s *Store) PartitionNames() []string {
	out := make([]string, 0, len(s.parts))
	for _, p := range s.parts {
		out = append(out, p.Name)
	}
	slices.Sort(out)
	return out
}

func (s *Store) Instance(guid string) *DumpInstance { return s.byGUID[strings.ToLower(guid)] }

func (s *Store) InstancesOfType(typ string) []*DumpInstance { return s.byType[typ] }

func (s *Store) Types() map[string]int {
	out := make(map[string]int, len(s.byType))
	for t, l := range s.byType {
		out[t] = len(l)
	}
	return out
}

func (s *Store) Resolve(from *DumpInstance, ref string) *DumpInstance {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "*nullGuid*" {
		return nil
	}
	if strings.HasPrefix(ref, "Instance") && from != nil {
		return from.Partition.Get(ref)
	}
	if len(ref) >= 36 {
		g := ref[len(ref)-36:]
		if len(ref) > 37 && ref[len(ref)-37] == '/' {
			if p := s.Partition(ref[:len(ref)-37]); p != nil {
				if in := p.Get(g); in != nil {
					return in
				}
			}
		}
		if in := s.Instance(g); in != nil {
			return in
		}
	}
	return nil
}

func (s *Store) String() string {
	if s.nFields == 0 {
		return fmt.Sprintf("ebx.Store(%d partitions, %d instances, %d types; fields parsed on demand)",
			len(s.parts), s.nInst, len(s.byType))
	}
	return fmt.Sprintf("ebx.Store(%d partitions, %d instances, %d types, %d fields)",
		len(s.parts), s.nInst, len(s.byType), s.nFields)
}

func (p *DumpPartition) Get(id string) *DumpInstance { return p.byID[strings.ToLower(id)] }

func (p *DumpPartition) Primary() *DumpInstance {
	if len(p.Instances) == 0 {
		return nil
	}
	return p.Instances[0]
}

func (p *DumpPartition) OfType(typ string) []*DumpInstance {
	var out []*DumpInstance
	for _, in := range p.Instances {
		if in.Type == typ {
			out = append(out, in)
		}
	}
	return out
}

func (in *DumpInstance) Is(typ string) bool {
	if in.Type == typ {
		return true
	}
	in.tree()
	for _, b := range in.Bases {
		if b == typ {
			return true
		}
	}
	return false
}

func (in *DumpInstance) Ref() string { return in.Partition.Name + "/" + in.ID }

func (in *DumpInstance) Node(path string) *DumpNode {
	n := in.tree()
	for el := range strings.SplitSeq(path, ".") {
		if el == "" {
			continue
		}
		if _, err := strconv.Atoi(el); err == nil {
			el = "member(" + el + ")"
		}
		n = findChild(n, el)
		if n == nil {
			return nil
		}
	}
	return n
}

func findChild(n *DumpNode, name string) *DumpNode {
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	for _, c := range n.Children {
		if c.Name == "$" {
			if f := findChild(c, name); f != nil {
				return f
			}
		}
	}
	return nil
}

func (in *DumpInstance) Field(path string) string {
	if n := in.Node(path); n != nil {
		return n.Value
	}
	return ""
}

func (in *DumpInstance) Float(path string) (float64, bool) {
	v, err := strconv.ParseFloat(in.Field(path), 64)
	return v, err == nil
}

func (in *DumpInstance) Vec3(path string) (v [3]float64, ok bool) {
	n := in.Node(path)
	if n == nil {
		return v, false
	}
	for i, k := range []string{"x", "y", "z"} {
		c := findChild(n, k)
		if c == nil {
			return v, false
		}
		f, err := strconv.ParseFloat(c.Value, 64)
		if err != nil {
			return v, false
		}
		v[i] = f
	}
	return v, true
}

func (in *DumpInstance) Members(path string) []*DumpNode {
	n := in.Node(path)
	if n == nil {
		return nil
	}
	return n.Children
}

type LinearTransform struct {
	Right, Up, Forward, Trans [3]float64
}

func (in *DumpInstance) Transform(path string) (t LinearTransform, ok bool) {
	var ok1, ok2, ok3, ok4 bool
	t.Right, ok1 = in.Vec3(path + ".right")
	t.Up, ok2 = in.Vec3(path + ".up")
	t.Forward, ok3 = in.Vec3(path + ".forward")
	t.Trans, ok4 = in.Vec3(path + ".trans")
	return t, ok1 && ok2 && ok3 && ok4
}

type SpawnPoint struct {
	GUID      string
	Team      string
	Priority  float64
	Enabled   bool
	Transform LinearTransform
}

func (s *Store) AlternateSpawns(partition string) []SpawnPoint {
	p := s.Partition(partition)
	if p == nil {
		return nil
	}
	var out []SpawnPoint
	for _, in := range p.OfType("AlternateSpawnEntityData") {
		t, ok := in.Transform("Transform")
		if !ok {
			continue
		}
		pr, _ := in.Float("Priority")
		out = append(out, SpawnPoint{
			GUID:      in.ID,
			Team:      in.Field("Team"),
			Priority:  pr,
			Enabled:   in.Field("Enabled") != "False",
			Transform: t,
		})
	}
	return out
}

func (s *Store) RegistryType(o RegistryObject) string {
	if in := s.Resolve(nil, o.Path+"/"+o.GUID.String()); in != nil {
		return in.Type
	}
	return ""
}
