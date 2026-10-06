package server

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Game struct {
	ID         uint32
	Name       string
	Level      string 
	Mode       string
	Attributes map[string]string
	MaxPlayers uint16
	Settings   uint32
	State      uint32
	Topology   uint32
	Version    string 
	HostID     uint64 
	HostIP     uint32
	HostPort   uint16
	Static     bool 
	Created    time.Time

	GameReg  uint32 
	HostName string
	HostAddr IpPairAddress
	HostPush func(b []byte) error 
	PGID     string

	GameType string 
	VOIP     uint32 

	players []*Player
}

type Player struct {
	PersonaID  uint64
	Name       string
	ExternalID uint64 
	Address    IpPairAddress
	Team       uint16
	Slot       uint8
	State      uint32
	Joined     time.Time
}

type PlayerInfo struct {
	PersonaID  uint64
	Name       string
	ExternalID uint64
	Address    IpPairAddress
}

var (
	ErrGameNotFound = errors.New("game not found")
	ErrGameFull     = errors.New("game is full")
)

type Registry struct {
	mu     sync.RWMutex
	nextID uint32
	games  map[uint32]*Game
}

func NewRegistry() *Registry {
	return &Registry{nextID: 1, games: map[uint32]*Game{}}
}

var Games = NewRegistry()

func (r *Registry) Add(g *Game) *Game {
	r.mu.Lock()
	defer r.mu.Unlock()

	g.ID = r.nextID
	r.nextID++
	if g.Attributes == nil {
		g.Attributes = map[string]string{}
	}
	if g.Level != "" {
		g.Attributes["level"] = g.Level
	}
	if g.Mode != "" {
		g.Attributes["mode"] = g.Mode
	}
	if g.Created.IsZero() {
		g.Created = time.Now()
	}
	r.games[g.ID] = g
	return g
}

func (r *Registry) CreateFromRequest(req CreateGameRequest, hostID uint64, fallbackIP uint32, fallbackPort uint16) *Game {
	g := &Game{
		Name:       req.GNAM,
		Attributes: copyAttrs(req.ATTR),
		MaxPlayers: req.PMAX,
		Settings:   req.GSET,
		State:      GameStateInitializing,
		Topology:   req.NTOP,
		Version:    req.VSTR,
		HostID:     hostID,
		HostIP:     fallbackIP,
		HostPort:   fallbackPort,
	}
	g.Level = g.Attributes["level"]
	g.Mode = g.Attributes["mode"]

	for _, a := range req.HNET {
		if a.VALU != nil {
			ip, port := a.VALU.EXIP.IP, a.VALU.EXIP.PORT
			if ip == 0 {
				ip, port = a.VALU.INIP.IP, a.VALU.INIP.PORT
			}
			if ip != 0 {
				g.HostIP, g.HostPort = ip, port
				break
			}
		}
	}
	if g.MaxPlayers == 0 {
		g.MaxPlayers = 24
	}
	if g.Name == "" {
		g.Name = fmt.Sprintf("Game %d", hostID)
	}
	return r.Add(g)
}

func (r *Registry) CreateFromBF4(req BF4CreateGameRequest, hostID uint64, hostName string, hostAddr IpPairAddress, push func([]byte) error) *Game {
	g := &Game{
		Name:       req.GNAM,
		Attributes: copyAttrs(req.ATTR),
		MaxPlayers: req.PMAX,
		Settings:   req.GSET,
		State:      GameStateInitializing,
		Topology:   req.NTOP,
		Version:    req.VSTR,
		HostID:     hostID,
		HostName:   hostName,
		HostAddr:   hostAddr,
		HostPush:   push,
		GameReg:    req.GMRG,
	}
	if len(req.PCAP) > 0 && req.PCAP[0] > 0 {
		g.MaxPlayers = req.PCAP[0]
	}
	if g.MaxPlayers == 0 {
		g.MaxPlayers = 64
	}
	if g.Version == "" {
		g.Version = ProtocolVersion
	}
	if g.Name == "" {
		g.Name = hostName
	}
	g.Level = g.Attributes["level"]
	g.Mode = g.Attributes["mode"]

	addr := hostAddr
	for _, a := range req.HNET {
		if a.VALU != nil && (a.VALU.EXIP.IP != 0 || a.VALU.INIP.IP != 0) {
			addr = *a.VALU
			break
		}
	}
	if addr.EXIP.IP == 0 {
		addr.EXIP = addr.INIP
	}
	g.HostAddr = addr
	g.HostIP, g.HostPort = addr.EXIP.IP, addr.EXIP.PORT

	g = r.Add(g)
	r.mu.Lock()
	g.PGID = fmt.Sprintf("%08x-%04x-4%03x-a%03x-%012x", g.ID, hostID&0xFFFF, g.ID&0xFFF, hostID&0xFFF, g.Created.UnixNano()&0xFFFFFFFFFFFF)
	r.mu.Unlock()
	return g
}

func (r *Registry) Get(id uint32) (*Game, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.games[id]
	return g, ok
}

func (r *Registry) List() []*Game {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Game, 0, len(r.games))
	for _, g := range r.games {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Registry) FindMatch(level, mode string) (*Game, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best *Game
	for _, g := range r.games {
		if g.Settings&SettingOpenToMatchmaking == 0 {
			continue
		}
		if g.State != GameStatePreGame && g.State != GameStateInGame {
			continue
		}
		if len(g.players) >= int(g.MaxPlayers) {
			continue
		}
		if level != "" && !strings.EqualFold(level, g.Level) {
			continue
		}
		if mode != "" && !strings.EqualFold(mode, g.Mode) {
			continue
		}
		if best == nil || len(g.players) > len(best.players) || (len(g.players) == len(best.players) && g.ID < best.ID) {
			best = g
		}
	}
	return best, best != nil
}

func (r *Registry) Join(id uint32, info PlayerInfo) (*Game, *Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	g, ok := r.games[id]
	if !ok {
		return nil, nil, ErrGameNotFound
	}
	for _, p := range g.players {
		if p.PersonaID == info.PersonaID {
			return g, p, nil
		}
	}
	if len(g.players) >= int(g.MaxPlayers) {
		return g, nil, ErrGameFull
	}

	var team0, team1 int
	for _, p := range g.players {
		if p.Team == 0 {
			team0++
		} else {
			team1++
		}
	}
	team := uint16(0)
	if team0 > team1 {
		team = 1
	}

	p := &Player{
		PersonaID:  info.PersonaID,
		Name:       info.Name,
		ExternalID: info.ExternalID,
		Address:    info.Address,
		Team:       team,
		Slot:       freeSlot(g),
		State:      PlayerStateActiveConnecting,
		Joined:     time.Now(),
	}
	g.players = append(g.players, p)
	return g, p, nil
}

func (r *Registry) Leave(id uint32, personaID uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.games[id]
	if !ok {
		return false
	}
	for i, p := range g.players {
		if p.PersonaID == personaID {
			g.players = append(g.players[:i], g.players[i+1:]...)
			return true
		}
	}
	return false
}

func (r *Registry) LeaveAll(personaID uint64) []uint32 {
	var left []uint32
	for _, g := range r.List() {
		if r.Leave(g.ID, personaID) {
			left = append(left, g.ID)
		}
	}
	return left
}

func (r *Registry) SetState(id uint32, state uint32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.games[id]
	if ok {
		g.State = state
	}
	return ok
}

func (r *Registry) SetAttributes(id uint32, attrs map[string]string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.games[id]
	if !ok {
		return false
	}
	for k, v := range attrs {
		g.Attributes[k] = v
	}
	g.Level = g.Attributes["level"]
	g.Mode = g.Attributes["mode"]
	return true
}

func (r *Registry) Destroy(id uint32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.games[id]
	if !ok || g.Static {
		return false
	}
	delete(r.games, id)
	return true
}

func (g *Game) hostAddress() []NetworkAddress {
	return []NetworkAddress{NewIpPairAddress(g.HostIP, g.HostPort)}
}

func (g *Game) teamCapacity() uint16 {
	if g.MaxPlayers < 2 {
		return g.MaxPlayers
	}
	return g.MaxPlayers / 2
}

func (r *Registry) BrowserData(g *Game) GameBrowserGameData {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roster := make([]GameBrowserPlayerData, 0, len(g.players))
	for _, p := range g.players {
		roster = append(roster, GameBrowserPlayerData{
			EXID: p.ExternalID, LOC: LocaleEnUS, NAME: p.Name, PATT: map[string]string{},
			PID: p.PersonaID, STAT: p.State, TIDX: p.Team,
		})
	}

	return GameBrowserGameData{
		ADMN: adminList(g),
		ATTR: copyAttrs(g.Attributes),
		CAP:  []uint16{g.MaxPlayers, 0},
		GID:  g.ID,
		GNAM: g.Name,
		GSET: g.Settings,
		GSTA: g.State,
		HNET: g.hostAddress(),
		HOST: g.HostID,
		NTOP: g.Topology,
		PCNT: []uint16{uint16(len(g.players)), 0},
		PRES: 1,
		PSAS: PingSite,
		QCAP: 0,
		QCNT: 0,
		ROST: roster,
		TCAP: g.teamCapacity(),
		VOIP: 1,
		VSTR: g.Version,
	}
}

func (r *Registry) ReplicatedData(g *Game) ReplicatedGameData {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return ReplicatedGameData{
		ADMN: adminList(g),
		ATTR: copyAttrs(g.Attributes),
		CAP:  []uint16{g.MaxPlayers, 0},
		GID:  g.ID,
		GNAM: g.Name,
		GSET: g.Settings,
		GSID: uint64(g.ID),
		GSTA: g.State,
		GTYP: "",
		HNET: g.hostAddress(),
		HSES: uint32(g.HostID),
		MCAP: g.MaxPlayers,
		NTOP: g.Topology,
		PHST: HostInfo{HPID: g.HostID},
		PRES: 1,
		PSAS: PingSite,
		SEED: g.ID * 2654435761,
		TCAP: g.teamCapacity(),
		THST: HostInfo{HPID: g.HostID},
		TIDS: []uint16{1, 2},
		UUID: fmt.Sprintf("bf4emu-%08x-%d", g.ID, g.Created.Unix()),
		VOIP: 1,
		VSTR: g.Version,
	}
}

func (r *Registry) Roster(g *Game) []ReplicatedGamePlayer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ReplicatedGamePlayer, 0, len(g.players))
	for _, p := range g.players {
		out = append(out, ReplicatedPlayer(g.ID, p))
	}
	return out
}

func ReplicatedPlayer(gameID uint32, p *Player) ReplicatedGamePlayer {
	addr := p.Address
	na := NetworkAddress{}
	_ = blazeUnionSet(&na, &addr)
	return ReplicatedGamePlayer{
		EXID: p.ExternalID,
		GID:  gameID,
		LOC:  LocaleEnUS,
		NAME: p.Name,
		PATT: map[string]string{},
		PID:  p.PersonaID,
		PNET: na,
		SID:  p.Slot,
		SLOT: 0,
		STAT: p.State,
		TIDX: p.Team,
		TIME: uint64(p.Joined.UnixMicro()),
		UID:  uint32(p.PersonaID),
	}
}

func (r *Registry) PlayerCount(g *Game) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(g.players)
}

func adminList(g *Game) []uint64 {
	if g.HostID == 0 {
		return []uint64{}
	}
	return []uint64{g.HostID}
}

func freeSlot(g *Game) uint8 {
	used := map[uint8]bool{}
	for _, p := range g.players {
		used[p.Slot] = true
	}
	for s := uint8(0); ; s++ {
		if !used[s] {
			return s
		}
	}
}

func copyAttrs(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (r *Registry) update(id uint32, fn func(g *Game)) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.games[id]
	if ok {
		fn(g)
	}
	return ok
}

func (r *Registry) SetSettings(id uint32, gset uint32) bool {
	return r.update(id, func(g *Game) { g.Settings = gset })
}

func (r *Registry) SetCapacity(id uint32, max uint16) bool {
	return r.update(id, func(g *Game) {
		if max > 0 {
			g.MaxPlayers = max
		}
	})
}

func (r *Registry) SetName(id uint32, name string) bool {
	return r.update(id, func(g *Game) { g.Name = name })
}

func (r *Registry) SetModRegister(id uint32, gmrg uint32) bool {
	return r.update(id, func(g *Game) { g.GameReg = gmrg })
}

func (r *Registry) SetPlayerState(id uint32, personaID uint64, state uint32) bool {
	found := false
	r.update(id, func(g *Game) {
		for _, p := range g.players {
			if p.PersonaID == personaID {
				p.State = state
				found = true
			}
		}
	})
	return found
}

func (r *Registry) FindPlayer(id uint32, personaID uint64) (Player, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if g, ok := r.games[id]; ok {
		for _, p := range g.players {
			if p.PersonaID == personaID {
				return *p, true
			}
		}
	}
	return Player{}, false
}

func (r *Registry) GamesOfHost(hostID uint64) []uint32 {
	var ids []uint32
	for _, g := range r.List() {
		if !g.Static && g.HostID == hostID {
			ids = append(ids, g.ID)
		}
	}
	return ids
}

func (r *Registry) HostPushFor(id uint32) func([]byte) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if g, ok := r.games[id]; ok {
		return g.HostPush
	}
	return nil
}

func (g *Game) HostPersona() uint64 {
	if g.HostID != 0 {
		return g.HostID
	}
	return 1000000 + uint64(g.ID)
}

func (g *Game) HostDisplayName() string {
	if g.HostName != "" {
		return g.HostName
	}
	return fmt.Sprintf("EMU-Host-%d", g.ID)
}

func (r *Registry) BF4Data(g *Game, joiner *Player) BF4GameData {
	r.mu.RLock()
	defer r.mu.RUnlock()

	phst := BF4HostInfo{HPID: g.HostPersona()}
	if joiner != nil && g.Topology != TopologyPeerHosted {
		slot := uint32(joiner.Slot) + 1
		phst = BF4HostInfo{CSID: slot, HPID: joiner.PersonaID, HSLT: uint8(slot)}
	}
	gtyp := g.GameType
	if gtyp == "" {
		gtyp = "frostbite_multiplayer"
	}
	pgid := g.PGID
	if pgid == "" {
		pgid = fmt.Sprintf("bf4emu-%08x-%d", g.ID, g.Created.Unix())
	}

	return BF4GameData{
		ADMN: []uint64{g.HostPersona()},
		ATTR: copyAttrs(g.Attributes),
		CAP:  []uint16{g.MaxPlayers, 0, 4, 0},
		GID:  g.ID,
		GMRG: g.GameReg,
		GNAM: g.Name,
		GPVH: 6667,
		GSET: g.Settings,
		GSID: 1337,
		GSTA: g.State,
		GTYP: gtyp,
		HNET: []NetworkAddress{gameHostAddress(g)},
		HSES: 13666,
		MCAP: 128,
		MNCP: 1,
		NQOS: NetworkQosData{NATT: 4},
		NRES: true,
		NTOP: g.Topology,
		PGID: pgid,
		PGSR: []byte{},
		PHST: phst,
		PRES: 1,
		PSAS: PingSite,
		QCAP: 14,
		SEED: 11181,
		THST: BF4HostInfo{HPID: g.HostPersona()},
		UUID: pgid,
		VOIP: g.VOIP,
		VSTR: g.Version,
		XNNC: []byte{},
		XSES: []byte{},
	}
}

func gameHostAddress(g *Game) NetworkAddress {
	if g.HostAddr.EXIP.IP != 0 || g.HostAddr.INIP.IP != 0 {
		pair := g.HostAddr
		if pair.INIP.IP == 0 {
			pair.INIP = pair.EXIP
		}
		na := NetworkAddress{}
		_ = blazeUnionSet(&na, &pair)
		return na
	}
	return NewIpPairAddress(g.HostIP, g.HostPort)
}

func BF4PlayerEntry(gameID uint32, p Player) BF4Player {
	addr := p.Address
	na := NetworkAddress{}
	_ = blazeUnionSet(&na, &addr)
	slot := uint32(p.Slot) + 1
	return BF4Player{
		BLOB: []byte{},
		CONG: p.PersonaID,
		CSID: slot,
		EXID: p.ExternalID,
		GID:  gameID,
		LOC:  LocaleEnUS,
		NAME: p.Name,
		PATT: map[string]string{"premium": "false"},
		PID:  p.PersonaID,
		PNET: na,
		ROLE: "soldier",
		SID:  slot,
		SLOT: 0,
		STAT: p.State,
		TIDX: p.Team,
		UID:  p.PersonaID,
	}
}

func (r *Registry) BF4Roster(g *Game) []BF4Player {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]BF4Player, 0, len(g.players))
	for _, p := range g.players {
		out = append(out, BF4PlayerEntry(g.ID, *p))
	}
	return out
}

func (r *Registry) PlayerIDs(id uint32) []uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.games[id]
	if !ok {
		return nil
	}
	out := make([]uint64, 0, len(g.players))
	for _, p := range g.players {
		out = append(out, p.PersonaID)
	}
	return out
}

type PeerGameOptions struct {
	Name       string
	Level      string
	Mode       string
	MaxPlayers uint16
	Settings   uint32
	GameType   string            
	VOIP       uint32            
	Attributes map[string]string 
}

func (r *Registry) CreatePeerGame(host PlayerInfo, o PeerGameOptions, push func([]byte) error) (*Game, *Player) {
	addr := host.Address
	if addr.EXIP.IP == 0 {
		addr.EXIP = addr.INIP
	}
	if addr.INIP.IP == 0 {
		addr.INIP = addr.EXIP
	}
	attrs := map[string]string{"mod": "DEFAULT"}
	for k, v := range o.Attributes {
		attrs[k] = v
	}
	g := r.Add(&Game{
		Name:       o.Name,
		Level:      o.Level,
		Mode:       o.Mode,
		MaxPlayers: o.MaxPlayers,
		Settings:   o.Settings,
		State:      GameStateInitializing,
		Topology:   TopologyPeerHosted,
		Version:    ProtocolVersion,
		HostID:     host.PersonaID,
		HostName:   host.Name,
		HostAddr:   addr,
		HostIP:     addr.EXIP.IP,
		HostPort:   addr.EXIP.PORT,
		HostPush:   push,
		GameType:   o.GameType,
		VOIP:       o.VOIP,
		Attributes: attrs,
	})
	r.mu.Lock()
	g.PGID = fmt.Sprintf("peer-%08x-%d", g.ID, g.Created.Unix())
	r.mu.Unlock()

	_, p, _ := r.Join(g.ID, host)
	return g, p
}

func (r *Registry) FindPeerMatch(level, mode string, exclude uint64) (*Game, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best *Game
	for _, g := range r.games {
		if g.Topology != TopologyPeerHosted || g.HostID == exclude {
			continue
		}
		if g.State == GameStateDestructing || g.State == GameStatePostGame {
			continue
		}
		if len(g.players) >= int(g.MaxPlayers) {
			continue
		}
		if level != "" && !strings.EqualFold(level, g.Level) {
			continue
		}
		if mode != "" && !strings.EqualFold(mode, g.Mode) {
			continue
		}
		if best == nil || len(g.players) > len(best.players) {
			best = g
		}
	}
	return best, best != nil
}

func (g *Game) IsPeerHosted() bool { return g.Topology == TopologyPeerHosted }

func (r *Registry) Players(id uint32) []Player {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.games[id]
	if !ok {
		return nil
	}
	out := make([]Player, 0, len(g.players))
	for _, p := range g.players {
		out = append(out, *p)
	}
	return out
}