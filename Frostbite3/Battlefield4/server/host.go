package server

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"bf4/logger"
)

var CaptureDir = "data/captures"

const HostPacketLogBytes = 64

type hostPeer struct {
	proto   string
	addr    string
	first   time.Time
	last    time.Time
	packets int
	bytes   int
}

type hostListener struct {
	port uint16

	mu    sync.Mutex
	peers map[string]*hostPeer

	capMu sync.Mutex
	cap   *os.File
}

var (
	hostOnce      sync.Once
	hostListeners []*hostListener
)

func StartHostListeners(r *Registry, extraPorts ...uint16) {
	hostOnce.Do(func() {
		capture := openCapture()

		ports := map[uint16]bool{}
		for _, g := range r.List() {
			if g.Static && g.HostPort != 0 {
				ports[g.HostPort] = true
			}
		}
		for _, p := range extraPorts {
			if p != 0 {
				ports[p] = true
			}
		}
		if len(ports) == 0 {
			logger.Warn("HOST: no listed games and no extra ports, nothing to listen on")
			return
		}

		var sorted []int
		for p := range ports {
			sorted = append(sorted, int(p))
		}
		sort.Ints(sorted)
		logger.Info("HOST: starting host listeners on ports %v (UDP + TCP)", sorted)

		for _, p := range sorted {
			h := &hostListener{port: uint16(p), peers: map[string]*hostPeer{}, cap: capture}
			hostListeners = append(hostListeners, h)
			go h.serveUDP()
			go h.serveTCP()
		}
		go reportLoop()
	})
}

func (h *hostListener) serveUDP() {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: int(h.port)})
	if err != nil {
		logger.Error("HOST: UDP :%d listen failed: %v", h.port, err)
		return
	}
	defer conn.Close()
	logger.Info("HOST: UDP listener active on :%d (waiting for the PS3 to join a game)", h.port)

	buf := make([]byte, 65535)
	for {
		n, peer, err := conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			logger.Warn("HOST: UDP :%d read error: %v", h.port, err)
			continue
		}
		h.packet("UDP", peer.String(), buf[:n])
	}
}

func (h *hostListener) serveTCP() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", h.port))
	if err != nil {
		logger.Error("HOST: TCP :%d listen failed: %v", h.port, err)
		return
	}
	defer ln.Close()
	logger.Info("HOST: TCP listener active on :%d", h.port)

	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			logger.Warn("HOST: TCP :%d accept error: %v", h.port, err)
			continue
		}
		go h.serveTCPConn(c)
	}
}

func (h *hostListener) serveTCPConn(c net.Conn) {
	defer c.Close()
	peer := c.RemoteAddr().String()
	logger.Info("HOST: TCP :%d connection from %s", h.port, peer)
	h.writeCapture("TCP-OPEN", peer, nil)

	buf := make([]byte, 65535)
	for {
		n, err := c.Read(buf)
		if n > 0 {
			h.packet("TCP", peer, buf[:n])
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				logger.Info("HOST: TCP :%d %s closed: %v", h.port, peer, err)
			} else {
				logger.Info("HOST: TCP :%d %s closed", h.port, peer)
			}
			h.writeCapture("TCP-CLOSE", peer, nil)
			return
		}
	}
}

func (h *hostListener) packet(proto, peer string, data []byte) {
	now := time.Now()
	key := proto + " " + peer

	h.mu.Lock()
	p, seen := h.peers[key]
	if !seen {
		p = &hostPeer{proto: proto, addr: peer, first: now}
		h.peers[key] = p
	}
	p.last = now
	p.packets++
	p.bytes += len(data)
	count := p.packets
	h.mu.Unlock()

	shown := data
	if len(shown) > HostPacketLogBytes {
		shown = shown[:HostPacketLogBytes]
	}

	if !seen {
		logger.Info("HOST: NEW %s peer %s -> :%d (first packet %d bytes)", proto, peer, h.port, len(data))
		logger.HexDump(logger.LevelInfo, fmt.Sprintf("HOST %s %s FIRST PACKET", proto, peer), data)
	} else if count <= 20 {
		logger.Debug("HOST: %s %s #%d %d bytes: %s", proto, peer, count, len(data), spacedHex(shown))
	} else {
		logger.Trace("HOST: %s %s #%d %d bytes: %s", proto, peer, count, len(data), spacedHex(shown))
	}

	h.writeCapture(proto, peer, data)
}

func (h *hostListener) writeCapture(kind, peer string, data []byte) {
	if h.cap == nil {
		return
	}
	h.capMu.Lock()
	defer h.capMu.Unlock()
	fmt.Fprintf(h.cap, "%s\t%s\t:%d\t%s\t%d\t%s\n",
		time.Now().Format("15:04:05.000000"), kind, h.port, peer, len(data), hex.EncodeToString(data))
}

func openCapture() *os.File {
	if err := os.MkdirAll(CaptureDir, 0o755); err != nil {
		logger.Warn("HOST: cannot create %s: %v (no capture file)", CaptureDir, err)
		return nil
	}
	path := filepath.Join(CaptureDir, "host_"+time.Now().Format("20060102_150405")+".tsv")
	f, err := os.Create(path)
	if err != nil {
		logger.Warn("HOST: cannot create %s: %v (no capture file)", path, err)
		return nil
	}
	fmt.Fprintln(f, "time\tproto\tport\tpeer\tlen\thex")
	logger.Info("HOST: capturing packets to %s", path)
	return f
}

func reportLoop() {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	last := map[string]int{}

	for range t.C {
		for _, h := range hostListeners {
			h.mu.Lock()
			for key, p := range h.peers {
				if last[key] == p.packets {
					continue
				}
				last[key] = p.packets
				logger.Info("HOST: :%d %s %s: %d packets, %d bytes since %s (last %s ago)",
					h.port, p.proto, p.addr, p.packets, p.bytes,
					p.first.Format("15:04:05"), time.Since(p.last).Round(time.Millisecond))
			}
			h.mu.Unlock()
		}
	}
}

func spacedHex(b []byte) string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, 0, len(b)*3)
	for i, v := range b {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, digits[v>>4], digits[v&0xF])
	}
	return string(out)
}