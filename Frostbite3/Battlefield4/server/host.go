package server

import (
	"bf4/server/dedicated"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		slices.Sort(sorted)
		logger.Info("HOST: starting host listeners on ports %v (UDP + TCP)", sorted)
		logger.Info("BUILD: %s", dedicated.BuildTag)
		if set := dedicated.EnvOverrides(); len(set) > 0 {
			logger.Info("BUILD: WARNING - BF4_* environment variables are set and override the defaults: %v (close this window or clear them to run the defaults)", set)
		} else {
			logger.Info("BUILD: no BF4_* environment variables set - running the defaults")
		}

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
		if h.port == QosPort {
			if reply := QosReply(buf[:n], peer); reply != nil {
				if _, err := conn.WriteToUDP(reply, peer); err != nil {
					logger.Warn("HOST: QoS reply to %s failed: %v", peer, err)
				}
			}
		}
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
			if h.port == QosPort {
				if resp := QosHTTP(buf[:n], c.RemoteAddr()); resp != nil {
					c.Write(resp)
					return
				}
			}
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

var QosPort uint16 = 17502

var qosReplies uint64

func QosReply(p []byte, peer *net.UDPAddr) []byte {
	if len(p) < 20 || (p[0]|p[1]|p[2]|p[3]) == 0 {
		return nil
	}
	ip4 := peer.IP.To4()
	if ip4 == nil {
		return nil
	}
	r := make([]byte, 30)
	copy(r, p[:20])

	if binary.BigEndian.Uint32(p[4:8]) >= 2 {
		n := qosCount(peer.IP.String(), p[0:12])
		binary.BigEndian.PutUint32(r[12:16], n)
	}
	copy(r[20:24], ip4)
	r[24] = byte(peer.Port >> 8)
	r[25] = byte(peer.Port)
	qosReplies++
	if qosReplies <= 5 || qosReplies%100 == 0 {
		logger.Info("HOST: QoS probe from %s (%d bytes, id %02x%02x%02x%02x) -> reply #%d", peer, len(p), p[0], p[1], p[2], p[3], qosReplies)
	}
	return r
}

var qosReqID uint32 = 1000

func QosHTTP(req []byte, from net.Addr) []byte {
	s := string(req)
	if !strings.HasPrefix(s, "GET /qos/") {
		return nil
	}
	line := s
	if i := strings.Index(line, "\r\n"); i >= 0 {
		line = line[:i]
	}
	path := strings.TrimPrefix(strings.Fields(line)[1], "/qos/")
	query := ""
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path, query = path[:i], path[i+1:]
	}
	q, _ := url.ParseQuery(query)
	qosReqID++
	id := qosReqID
	secret := id*2654435761 | 1
	var body string
	switch path {
	case "qos":
		probes, size := 10, 20
		if q.Get("qtyp") == "2" {
			probes, size = 10, 1200
		} else {
			id = 1
		}
		body = fmt.Sprintf("<qos><numprobes>%d</numprobes><probesize>%d</probesize><qosport>%d</qosport><requestid>%d</requestid><reqsecret>%d</reqsecret></qos>",
			probes, size, QosPort, id, secret)
	case "firewall":
		body = fmt.Sprintf("<firewall><numinterfaces>1</numinterfaces><ips><ips>%d</ips></ips><ports><ports>%d</ports></ports><requestid>%d</requestid><reqsecret>%d</reqsecret></firewall>",
			QosPublicIP, QosPort, id, secret)
	case "firetype":
		body = "<firetype><firetype>0</firetype></firetype>"
	default:
		return nil
	}
	logger.Info("HOST: QoS HTTP %s from %s -> %s", line, from, body)
	return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/xml\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body))
}

var QosPublicIP uint32 = 151<<24 | 244<<16 | 72<<8 | 66

var (
	qosMu     sync.Mutex
	qosCounts = map[string]uint32{}
)

func qosCount(peer string, hdr []byte) uint32 {
	key := peer + "/" + string(hdr)
	qosMu.Lock()
	defer qosMu.Unlock()
	if len(qosCounts) > 4096 {
		qosCounts = map[string]uint32{}
	}
	qosCounts[key]++
	return qosCounts[key]
}
