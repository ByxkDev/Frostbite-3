package dedicated

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"bf4/blaze"
	"bf4/logger"
)

type Config struct {
	Blaze       string        // Blaze game port of the emulator, e.g. "0.0.0.0:33152"
	IP          string        // IPv4 players connect to
	Port        int           // game port players connect to (UDP + TCP)
	Name        string        // server name in the browser
	Level       string        // LevelName, e.g. "Levels/MP/MP_Siege/MP_Siege"
	Mode        string        // TeamDeathMatch0, ConquestLarge0, ...
	Mod         string        // DEFAULT, XPACK0..XPACK7
	Players     int           // soldier slots, max 64
	Spectators  int           // spectator slots
	Queue       int           // queue capacity
	Mail        string        // server account mail
	Password    string        // server account password, if the emulator requires one
	Persona     string        // server persona name
	Version     string        // game protocol version (the PS3's GVER)
	JoinTimeout time.Duration // how long a joining player may take to connect
	FakeConnect bool          // TESTING: report players connected on their first packet
	CaptureDir  string        // folder for packet captures
	Engine      GameEngine    // nil = CaptureEngine
}

func DefaultConfig() Config {
	return Config{
		Blaze:       "0.0.0.0:33152",
		IP:          "0.0.0.0",
		Port:        25210,
		Name:        "[DS] Siege of Shanghai - Team Deathmatch",
		Level:       "Levels/MP/MP_Siege/MP_Siege",
		Mode:        "TeamDeathMatch0",
		Mod:         "DEFAULT",
		Players:     64,
		Spectators:  4,
		Queue:       10,
		Mail:        "bf4.server.ps3@emu.local",
		Persona:     "BF4-DS-1",
		Version:     "5900",
		JoinTimeout: 20 * time.Second,
		CaptureDir:  "data/captures",
	}
}

type Server struct {
	cfg    Config
	ip     uint32
	engine GameEngine

	udp *net.UDPConn
	tcp net.Listener

	mu      sync.Mutex
	b       *blazeConn
	gameID  uint32
	players map[uint64]*player
	stopped bool
	done    chan struct{}
}

type player struct {
	ID        uint64
	Name      string
	IP        uint32
	Port      uint16
	Joined    time.Time
	Seen      bool
	Connected bool
}

func Start(cfg Config) (*Server, error) {
	if cfg.Players < 1 || cfg.Players > 64 {
		return nil, fmt.Errorf("players must be 1..64 (BF4 soldier role capacity), got %d", cfg.Players)
	}
	ip, err := parseIPv4(cfg.IP)
	if err != nil {
		return nil, err
	}
	if cfg.JoinTimeout <= 0 {
		cfg.JoinTimeout = 20 * time.Second
	}

	s := &Server{cfg: cfg, ip: ip, engine: cfg.Engine, players: map[uint64]*player{}, done: make(chan struct{})}
	if s.engine == nil {
		s.engine = NewCaptureEngine(cfg.CaptureDir)
	}

	logger.Info("DEDICATED: %q  %s / %s  %d players  game port %s:%d  Blaze %s",
		cfg.Name, cfg.Level, cfg.Mode, cfg.Players, cfg.IP, cfg.Port, cfg.Blaze)

	if err := s.openGamePort(); err != nil {
		return nil, fmt.Errorf("game port :%d: %w", cfg.Port, err)
	}

	go s.loop()
	return s, nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	b, gid := s.b, s.gameID
	s.mu.Unlock()

	if b != nil && gid != 0 {
		logger.Info("DEDICATED: stopping, destroying game %d", gid)
		_, _ = b.Call(gmComponent, gmDestroyGame, &destroyGameRequest{GID: gid}, 5*time.Second)
	}
	if b != nil {
		b.Close()
	}
	if s.udp != nil {
		s.udp.Close()
	}
	if s.tcp != nil {
		s.tcp.Close()
	}
	<-s.done
}

func (s *Server) GameID() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gameID
}

func (s *Server) isStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

func (s *Server) loop() {
	defer close(s.done)
	delay := 2 * time.Second
	for attempt := 1; !s.isStopped(); attempt++ {
		err := s.session()
		if s.isStopped() {
			return
		}
		logger.Warn("DEDICATED: Blaze session ended: %v -- reconnecting in %s (attempt %d)", err, delay, attempt)
		time.Sleep(delay)
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

func (s *Server) session() error {
	b, err := dialBlaze(s.cfg.Blaze)
	if err != nil {
		return err
	}
	defer b.Close()

	s.mu.Lock()
	s.b = b
	s.gameID = 0
	s.players = map[uint64]*player{}
	s.mu.Unlock()

	go s.handleNotifications(b)

	if err := s.login(b); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if err := s.createGame(b); err != nil {
		return fmt.Errorf("create game: %w", err)
	}

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	check := time.NewTicker(time.Second)
	defer check.Stop()

	for {
		select {
		case <-b.Done():
			return b.Err()
		case <-ping.C:
			if _, err := b.Call(utilComponent, utilPing, nil, 10*time.Second); err != nil {
				return fmt.Errorf("ping: %w", err)
			}
		case <-check.C:
			s.expireJoins()
		}
	}
}

func (s *Server) login(b *blazeConn) error {
	if _, err := b.Call(utilComponent, utilPreAuth, &preAuthRequest{
		CINF: clientInfo{
			BSDK: "13.3.1.2.1",
			BTIM: "Oct 17 2015 22:07:08",
			CLNT: "warsaw server",
			CPFT: 2, // PS3
			CSKU: "ps3",
			CVER: "Warsaw176598retail-1.0-" + s.cfg.Version,
			DSDK: "13.3.1.2.1",
			ENV:  "prod",
		},
		FCCR: fetchConfig{CFID: "BlazeSDK"},
	}, 15*time.Second); err != nil {
		return fmt.Errorf("PreAuth: %w", err)
	}

	if _, err := b.Call(authComponent, authLogin, &loginRequest{MAIL: s.cfg.Mail, PASS: s.cfg.Password}, 15*time.Second); err != nil {
		return fmt.Errorf("Login: %w", err)
	}
	if _, err := b.Call(authComponent, authLoginPersona, &loginPersonaRequest{PNAM: s.cfg.Persona}, 15*time.Second); err != nil {
		return fmt.Errorf("LoginPersona: %w", err)
	}
	if _, err := b.Call(utilComponent, utilPostAuth, nil, 15*time.Second); err != nil {
		return fmt.Errorf("PostAuth: %w", err)
	}
	if _, err := b.Call(usComponent, usUpdateNetworkInfo, &updateNetworkInfoRequest{ADDR: s.address()}, 15*time.Second); err != nil {
		return fmt.Errorf("UpdateNetworkInfo: %w", err)
	}

	logger.Info("DEDICATED: logged in to Blaze as \"warsaw server\" (%s / %q), game port %s:%d",
		s.cfg.Mail, s.cfg.Persona, s.cfg.IP, s.cfg.Port)
	return nil
}

func (s *Server) address() networkAddress {
	pair := ipPairAddress{
		EXIP: ipAddress{IP: s.ip, PORT: uint16(s.cfg.Port)},
		INIP: ipAddress{IP: s.ip, PORT: uint16(s.cfg.Port)},
	}
	return newNetworkAddress(pair)
}

func (s *Server) createGame(b *blazeConn) error {
	settings := settingOpenToInvites | settingOpenToMatchmaking | settingOpenToJoinByPlayer |
		settingOpenToBrowsing | settingRanked | settingJoinInProgress

	reply, err := b.Call(gmComponent, gmCreateGame, &createGameRequest{
		ATTR: map[string]string{"level": s.cfg.Level, "mode": s.cfg.Mode, "mod": s.cfg.Mod},
		GNAM: s.cfg.Name,
		GSET: settings,
		GTYP: "frostbite_multiplayer",
		HNET: []networkAddress{s.address()},
		NRES: true,
		NTOP: topologyDedicatedServer,
		PCAP: []uint16{uint16(s.cfg.Players), 0, uint16(s.cfg.Spectators), 0},
		PMAX: uint16(s.cfg.Players),
		QCAP: uint16(s.cfg.Queue),
		RCAP: map[string]uint16{"soldier": uint16(s.cfg.Players), "commander": 2},
		VOIP: 0,
		VSTR: s.cfg.Version,
	}, 15*time.Second)
	if err != nil {
		return err
	}

	var resp createGameResponse
	if err := tdfDecoder.Decode(reply.Payload, &resp); err != nil || resp.GID == 0 {
		return fmt.Errorf("bad createGame reply (%v, GID=%d)", err, resp.GID)
	}

	s.mu.Lock()
	s.gameID = resp.GID
	s.mu.Unlock()

	if _, err := b.Call(gmComponent, gmFinalizeGameCreation, &gameIDRequest{GID: resp.GID}, 15*time.Second); err != nil {
		return fmt.Errorf("finalizeGameCreation: %w", err)
	}
	for _, state := range []uint32{gameStatePreGame, gameStateInGame} {
		if _, err := b.Call(gmComponent, gmAdvanceGameState, &advanceGameStateRequest{GID: resp.GID, GSTA: state}, 15*time.Second); err != nil {
			return fmt.Errorf("advanceGameState %d: %w", state, err)
		}
	}

	logger.Info("DEDICATED: game %d %q (%s / %s) is IN_GAME and open, engine: %s",
		resp.GID, s.cfg.Name, s.cfg.Level, s.cfg.Mode, s.engine.Name())
	return nil
}

func (s *Server) handleNotifications(b *blazeConn) {
	for n := range b.Notifications() {
		switch {
		case n.Component == gmComponent && (n.Command == gmNotifyPlayerJoining || n.Command == gmNotifyPlayerClaimingReservation):
			var msg playerJoiningNotify
			_ = tdfDecoder.Decode(n.Payload, &msg)
			s.playerJoining(msg)

		case n.Component == gmComponent && n.Command == gmNotifyPlayerRemoved:
			var msg playerRemovedNotify
			_ = tdfDecoder.Decode(n.Payload, &msg)
			s.mu.Lock()
			if p, ok := s.players[msg.PID]; ok {
				logger.Info("DEDICATED: %q (%d) removed from the game (reason %d)", p.Name, p.ID, msg.REAS)
				delete(s.players, msg.PID)
			}
			s.mu.Unlock()

		case n.Component == gmComponent && n.Command == gmNotifyGameRemoved:
			logger.Info("DEDICATED: Blaze removed the game")
		}
	}
}

func (s *Server) playerJoining(msg playerJoiningNotify) {
	p := msg.PDAT
	ip, port := uint32(0), uint16(0)
	if p.PNET.VALU != nil {
		ip, port = p.PNET.VALU.EXIP.IP, p.PNET.VALU.EXIP.PORT
		if ip == 0 {
			ip, port = p.PNET.VALU.INIP.IP, p.PNET.VALU.INIP.PORT
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.players[p.PID]; ok {
		return 
	}
	s.players[p.PID] = &player{ID: p.PID, Name: p.NAME, IP: ip, Port: port, Joined: time.Now()}
	logger.Info("DEDICATED: %q (%d) is joining from %s:%d, team %d -- waiting for their connection on :%d",
		p.NAME, p.PID, ipString(ip), port, p.TIDX, s.cfg.Port)
}

func (s *Server) playerByAddr(addr *net.UDPAddr) *player {
	ip4 := addr.IP.To4()
	if ip4 == nil {
		return nil
	}
	ip := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.players {
		if p.IP == ip {
			return p
		}
	}
	return nil
}

func (s *Server) reportConnection(p *player, connected bool) {
	s.mu.Lock()
	b, gid := s.b, s.gameID
	if connected {
		p.Connected = true
	} else {
		delete(s.players, p.ID)
	}
	s.mu.Unlock()
	if b == nil {
		return
	}

	stat := meshDisconnected
	if connected {
		stat = meshConnected
	}
	_, err := b.Call(gmComponent, gmUpdateMeshConnection, &updateMeshConnectionRequest{
		GID:  gid,
		STAT: stat,
		TCG:  blaze.NewBlazeObjectID(int64(p.ID), blaze.NewBlazeObjectType(usComponent, 1)),
	}, 10*time.Second)
	if err != nil {
		logger.Warn("DEDICATED: updateMeshConnection for %q failed: %v", p.Name, err)
		return
	}
	if connected {
		logger.Info("DEDICATED: reported %q (%d) as CONNECTED", p.Name, p.ID)
	} else {
		logger.Info("DEDICATED: reported %q (%d) as NOT connected", p.Name, p.ID)
	}
}

func (s *Server) expireJoins() {
	var expired []*player
	s.mu.Lock()
	for _, p := range s.players {
		if !p.Connected && time.Since(p.Joined) > s.cfg.JoinTimeout {
			expired = append(expired, p)
		}
	}
	s.mu.Unlock()

	for _, p := range expired {
		if p.Seen {
			logger.Info("DEDICATED: %q reached the game port but the engine (%s) did not complete the connection within %s",
				p.Name, s.engine.Name(), s.cfg.JoinTimeout)
		} else {
			logger.Warn("DEDICATED: %q never reached game port :%d within %s -- check the firewall", p.Name, s.cfg.Port, s.cfg.JoinTimeout)
		}
		s.reportConnection(p, false)
	}
}

func (s *Server) openGamePort() error {
	udp, err := net.ListenUDP("udp", &net.UDPAddr{Port: s.cfg.Port})
	if err != nil {
		return err
	}
	tcp, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.Port))
	if err != nil {
		udp.Close()
		return err
	}
	s.udp, s.tcp = udp, tcp
	logger.Info("DEDICATED: game port :%d open (UDP + TCP)", s.cfg.Port)

	go func() {
		buf := make([]byte, 65535)
		for {
			n, from, err := udp.ReadFromUDP(buf)
			if err != nil {
				if !s.isStopped() {
					logger.Warn("DEDICATED: game port UDP: %v", err)
				}
				return
			}
			s.gamePacket(from, append([]byte(nil), buf[:n]...))
		}
	}()

	go func() {
		for {
			c, err := tcp.Accept()
			if err != nil {
				return
			}
			logger.Info("DEDICATED: TCP connection on the game port from %s (BF4 game traffic is UDP; capturing anyway)", c.RemoteAddr())
			go func() {
				defer c.Close()
				buf := make([]byte, 65535)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						s.engine.Capture("TCP", c.RemoteAddr().String(), buf[:n])
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	return nil
}

func (s *Server) gamePacket(from *net.UDPAddr, data []byte) {
	p := s.playerByAddr(from)
	name := "unknown"
	if p != nil {
		name = p.Name
		s.mu.Lock()
		first := !p.Seen
		p.Seen = true
		s.mu.Unlock()
		if first {
			logger.Info("DEDICATED: %q reached the game port from %s (%d-byte packet)", p.Name, from, len(data))
		}
	}

	replies, connected := s.engine.HandlePacket(from, name, data)
	for _, r := range replies {
		if _, err := s.udp.WriteToUDP(r, from); err != nil {
			logger.Warn("DEDICATED: send to %s: %v", from, err)
		}
	}

	if p == nil {
		return
	}
	s.mu.Lock()
	already := p.Connected
	s.mu.Unlock()
	if already {
		return
	}
	if connected || s.cfg.FakeConnect {
		if !connected {
			logger.Warn("DEDICATED: FakeConnect: reporting %q as connected although the engine did not connect them", p.Name)
		}
		go s.reportConnection(p, true)
	}
}

func parseIPv4(v string) (uint32, error) {
	ip := net.ParseIP(strings.TrimSpace(v)).To4()
	if ip == nil {
		return 0, fmt.Errorf("bad IPv4 address %q", v)
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3]), nil
}

func ipString(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}
