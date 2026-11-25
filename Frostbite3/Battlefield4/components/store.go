package components

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SocketID uint8

func (id SocketID) String() string { return fmt.Sprintf("0x%02X", uint8(id)) }

var (
	ErrSocketInUse    = errors.New("socket: id already open")
	ErrSocketNotFound = errors.New("socket: id not open")
	ErrSocketClosed   = errors.New("socket: closed")
)

type SocketStats struct {
	ID          SocketID  `json:"id"`
	Name        string    `json:"name"`
	Addr        string    `json:"addr"`
	OpenedAt    time.Time `json:"openedAt"`
	ActiveConns int       `json:"activeConns"`
	TotalConns  uint64    `json:"totalConns"`
	BytesIn     uint64    `json:"bytesIn"`
	BytesOut    uint64    `json:"bytesOut"`
}

type SocketManager struct {
	mu      sync.RWMutex
	sockets map[SocketID]*Socket
	logger  *log.Logger
}

func NewSocketManager(logger *log.Logger) *SocketManager {
	if logger == nil {
		logger = log.Default()
	}
	return &SocketManager{sockets: make(map[SocketID]*Socket), logger: logger}
}

func (m *SocketManager) Open(id SocketID, name, addr string) (*Socket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sockets[id]; exists {
		return nil, fmt.Errorf("%w: %s", ErrSocketInUse, id)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("socket %s (%s): listen %s: %w", id, name, addr, err)
	}

	s := &Socket{
		id:       id,
		name:     name,
		ln:       ln,
		manager:  m,
		logger:   m.logger,
		conns:    make(map[uint64]*SocketConn),
		openedAt: time.Now().UTC(),
	}
	m.sockets[id] = s
	m.logger.Printf("[socket %s] opened %q on %s", id, name, ln.Addr())
	return s, nil
}

func (m *SocketManager) Get(id SocketID) (*Socket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sockets[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSocketNotFound, id)
	}
	return s, nil
}

func (m *SocketManager) Close(id SocketID) error {
	s, err := m.Get(id)
	if err != nil {
		return err
	}
	return s.Close()
}

func (m *SocketManager) CloseAll() {
	m.mu.RLock()
	all := make([]*Socket, 0, len(m.sockets))
	for _, s := range m.sockets {
		all = append(all, s)
	}
	m.mu.RUnlock()

	for _, s := range all {
		_ = s.Close()
	}
}

func (m *SocketManager) Stats() []SocketStats {
	m.mu.RLock()
	out := make([]SocketStats, 0, len(m.sockets))
	for _, s := range m.sockets {
		out = append(out, s.Stats())
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *SocketManager) remove(id SocketID) {
	m.mu.Lock()
	delete(m.sockets, id)
	m.mu.Unlock()
}

type Socket struct {
	id       SocketID
	name     string
	ln       net.Listener
	manager  *SocketManager
	logger   *log.Logger
	openedAt time.Time

	mu    sync.Mutex
	conns map[uint64]*SocketConn

	nextConn   atomic.Uint64
	totalConns atomic.Uint64
	bytesIn    atomic.Uint64
	bytesOut   atomic.Uint64

	closeOnce sync.Once
	closed    atomic.Bool
	trace     atomic.Bool
}

func (s *Socket) SetTrace(on bool) { s.trace.Store(on) }

const socketTraceMax = 2048

func (s *Socket) traceData(connID uint64, dir string, p []byte) {
	if !s.trace.Load() || len(p) == 0 {
		return
	}
	shown := p
	suffix := ""
	if len(shown) > socketTraceMax {
		shown = shown[:socketTraceMax]
		suffix = fmt.Sprintf(" ...(+%d bytes)", len(p)-socketTraceMax)
	}
	s.logger.Printf("[socket %s] conn=%d %s %d bytes: %q%s", s.id, connID, dir, len(p), shown, suffix)
}

func (s *Socket) ID() SocketID { return s.id }

func (s *Socket) Name() string { return s.name }

func (s *Socket) Accept() (net.Conn, error) {
	c, err := s.ln.Accept()
	if err != nil {
		if s.closed.Load() {
			return nil, net.ErrClosed
		}
		return nil, err
	}

	sc := &SocketConn{
		Conn:        c,
		socket:      s,
		id:          s.nextConn.Add(1),
		connectedAt: time.Now().UTC(),
	}

	s.mu.Lock()
	s.conns[sc.id] = sc
	s.mu.Unlock()
	s.totalConns.Add(1)

	s.logger.Printf("[socket %s] conn=%d accepted from %s", s.id, sc.id, c.RemoteAddr())
	return sc, nil
}

func (s *Socket) Addr() net.Addr { return s.ln.Addr() }

func (s *Socket) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		err = s.ln.Close()

		s.mu.Lock()
		live := make([]*SocketConn, 0, len(s.conns))
		for _, c := range s.conns {
			live = append(live, c)
		}
		s.mu.Unlock()
		for _, c := range live {
			_ = c.Close()
		}

		s.manager.remove(s.id)
		s.logger.Printf("[socket %s] closed %q", s.id, s.name)
	})
	if errors.Is(err, net.ErrClosed) {
		err = nil
	}
	return err
}

func (s *Socket) Conns() []*SocketConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*SocketConn, 0, len(s.conns))
	for _, c := range s.conns {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

func (s *Socket) Stats() SocketStats {
	s.mu.Lock()
	active := len(s.conns)
	s.mu.Unlock()
	return SocketStats{
		ID:          s.id,
		Name:        s.name,
		Addr:        s.ln.Addr().String(),
		OpenedAt:    s.openedAt,
		ActiveConns: active,
		TotalConns:  s.totalConns.Load(),
		BytesIn:     s.bytesIn.Load(),
		BytesOut:    s.bytesOut.Load(),
	}
}

func (s *Socket) untrack(id uint64) {
	s.mu.Lock()
	delete(s.conns, id)
	s.mu.Unlock()
}

type SocketConn struct {
	net.Conn
	socket      *Socket
	id          uint64
	connectedAt time.Time
	closeOnce   sync.Once
	bytesIn     atomic.Uint64
	bytesOut    atomic.Uint64
}

func (c *SocketConn) ID() uint64 { return c.id }

func (c *SocketConn) Socket() *Socket { return c.socket }

func (c *SocketConn) ConnectedAt() time.Time { return c.connectedAt }

func (c *SocketConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.socket.bytesIn.Add(uint64(n))
		c.bytesIn.Add(uint64(n))
		c.socket.traceData(c.id, "IN", p[:n])
	}
	return n, err
}

func (c *SocketConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.socket.bytesOut.Add(uint64(n))
		c.bytesOut.Add(uint64(n))
		c.socket.traceData(c.id, "OUT", p[:n])
	}
	return n, err
}

func (c *SocketConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.Conn.Close()
		c.socket.untrack(c.id)
		c.socket.logger.Printf("[socket %s] conn=%d closed (in=%d bytes, out=%d bytes, open %s)",
			c.socket.id, c.id, c.bytesIn.Load(), c.bytesOut.Load(),
			time.Since(c.connectedAt).Round(time.Millisecond))
	})
	return err
}

const (
	StoreSocketID   SocketID = 0x27
	StoreSocketName          = "dice-commerce"
	StoreAddr                = ":80"
	StoreRoute               = "/dice-commerce/battlefield4/"
)

var StoreTrace = true

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func (s *Store) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var connID uint64
		if sc, ok := r.Context().Value(storeConnKey{}).(*SocketConn); ok {
			connID = sc.ID()
		}
		s.logger.Printf("[store %s] conn=%d REQUEST %s %s %s Host=%q UA=%q",
			StoreSocketID, connID, r.Method, r.RequestURI, r.Proto, r.Host, r.UserAgent())
		for name, values := range r.Header {
			for _, v := range values {
				s.logger.Printf("[store %s] conn=%d   %s: %s", StoreSocketID, connID, name, v)
			}
		}

		rec := &statusRecorder{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		s.logger.Printf("[store %s] conn=%d RESPONSE %d (%d bytes) in %s",
			StoreSocketID, connID, rec.status, rec.bytes, time.Since(start).Round(time.Microsecond))
	})
}

const CDNDir = "data/cdn"

func cdnFile(urlPath string) string {
	clean := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	if clean == "" || clean == "." {
		return ""
	}
	p := filepath.Join(CDNDir, filepath.FromSlash(clean))
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

func (s *Store) handleOther(w http.ResponseWriter, r *http.Request) {
	if f := cdnFile(r.URL.Path); f != "" {
		s.logger.Printf("[store %s] CDN file %q -> %s", StoreSocketID, r.URL.Path, f)
		http.ServeFile(w, r, f)
		return
	}
	s.logger.Printf("[store %s] CDN file missing: %q (put the real file at %s)", StoreSocketID, r.URL.Path,
		filepath.Join(CDNDir, filepath.FromSlash(strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/"))))
	http.NotFound(w, r)
}

type StoreItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Price    int    `json:"price"`
	Currency string `json:"currency"`
}

type StoreResponse struct {
	Game     string      `json:"game"`
	Socket   string      `json:"socket"`
	ConnID   uint64      `json:"connId"`
	Items    []StoreItem `json:"items"`
	ServedAt time.Time   `json:"servedAt"`
}

type storeConnKey struct{}

type Store struct {
	sockets *SocketManager
	addr    string
	logger  *log.Logger

	mu      sync.RWMutex
	catalog []StoreItem

	handler http.Handler
	server  *http.Server
	socket  atomic.Pointer[Socket]
}

const StorePrefix = "/dice-commerce/"

func (s *Store) Handler() http.Handler { return s.handler }

func (s *Store) Wrap(next http.Handler) http.Handler {
	if next == nil {
		next = http.DefaultServeMux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, StorePrefix) {
			s.handler.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func NewStore(sockets *SocketManager, logger *log.Logger, catalog []StoreItem) *Store {
	return NewStoreOnAddr(sockets, logger, catalog, StoreAddr)
}

func NewStoreOnAddr(sockets *SocketManager, logger *log.Logger, catalog []StoreItem, addr string) *Store {
	if logger == nil {
		logger = log.Default()
	}
	if sockets == nil {
		sockets = NewSocketManager(logger)
	}
	s := &Store{sockets: sockets, addr: addr, logger: logger, catalog: catalog}

	mux := http.NewServeMux()
	mux.HandleFunc(StoreRoute, s.handleStore)
	mux.HandleFunc("/", s.handleOther)
	s.handler = s.logRequests(mux)

	s.server = &http.Server{
		Handler:           s.handler,
		ErrorLog:          logger,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			if sc, ok := c.(*SocketConn); ok {
				return context.WithValue(ctx, storeConnKey{}, sc)
			}
			return ctx
		},
	}
	return s
}

func (s *Store) SetCatalog(items []StoreItem) {
	s.mu.Lock()
	s.catalog = items
	s.mu.Unlock()
}

func (s *Store) Socket() *Socket { return s.socket.Load() }

func (s *Store) Start() error {
	sock, err := s.sockets.Open(StoreSocketID, StoreSocketName, s.addr)
	if err != nil {
		return err
	}
	s.socket.Store(sock)
	sock.SetTrace(StoreTrace)

	s.logger.Printf("[store %s] serving GET %s on %s", StoreSocketID, StoreRoute, sock.Addr())
	err = s.server.Serve(sock)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (s *Store) Shutdown(ctx context.Context) error {
	s.logger.Printf("[store %s] shutting down", StoreSocketID)
	err := s.server.Shutdown(ctx)
	if sock := s.socket.Load(); sock != nil {
		_ = sock.Close()
	}
	return err
}

func (s *Store) handleStore(w http.ResponseWriter, r *http.Request) {
	var connID uint64
	if sc, ok := r.Context().Value(storeConnKey{}).(*SocketConn); ok {
		connID = sc.ID()
	}

	if r.URL.Path != StoreRoute {
		s.handleOther(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.logger.Printf("[store %s] conn=%d %s %s from %s",
		StoreSocketID, connID, r.Method, r.URL.Path, r.RemoteAddr)

	s.mu.RLock()
	items := make([]StoreItem, len(s.catalog))
	copy(items, s.catalog)
	s.mu.RUnlock()

	resp := StoreResponse{
		Game:     "battlefield4",
		Socket:   StoreSocketID.String(),
		ConnID:   connID,
		Items:    items,
		ServedAt: time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Store-Socket", resp.Socket)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Printf("[store %s] conn=%d encode error: %v", StoreSocketID, connID, err)
	}
}

func DefaultCatalog() []StoreItem {
	return []StoreItem{
		{ID: "bf4-shortcut-weapons", Name: "Weapon Shortcut Kit", Category: "shortcut", Price: 1500, Currency: "BF4"},
		{ID: "bf4-battlepack-gold", Name: "Gold Battlepack", Category: "battlepack", Price: 500, Currency: "BF4"},
		{ID: "bf4-camo-urban", Name: "Urban Camo", Category: "cosmetic", Price: 250, Currency: "BF4"},
	}
}
