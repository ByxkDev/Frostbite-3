package components

import (
	"bytes"
	"net"
	"sync"
	"sync/atomic"

	"bf4/blaze"
	"bf4/logger"
	"bf4/server"
)

type Conn struct {
	ID     uint64
	Remote string               
	Push   func(b []byte) error 

	mu        sync.Mutex
	host      *hostSession
	pushToken uint64
}

var nextConnID uint64

func NewConn(remote string, push func(b []byte) error) *Conn {
	return &Conn{ID: atomic.AddUint64(&nextConnID, 1), Remote: remote, Push: push}
}

func (c *Conn) IsHost() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.host != nil
}

func (c *Conn) RemoteIP() uint32 {
	host, _, err := net.SplitHostPort(c.Remote)
	if err != nil {
		host = c.Remote
	}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func HandleConnPacket(c *Conn, data []byte) []byte {
	p := blaze.Parse(data)

	if !c.IsHost() && isHostHello(p) {
		c.becomeHost()
	}

	c.mu.Lock()
	h := c.host
	c.mu.Unlock()

	if h != nil {
		defer logger.Timed("HOST HandlePacket")()
		logger.Request(data)
		resp := handleHostPacket(c, h, p)
		if len(resp) > 0 {
			logger.Response(resp)
		}
		return resp
	}

	c.ensureClientPush()
	return HandlePacket(data)
}

func ConnClosed(c *Conn) {
	c.mu.Lock()
	h := c.host
	token := c.pushToken
	c.mu.Unlock()

	if h != nil {
		hostClosed(c, h)
		return
	}

	ClearPushNotification(token)
	if id := currentPersonaID(); id != 0 {
		if left := server.Games.LeaveAll(id); len(left) > 0 {
			logger.Info("CONN %d: PS3 player %d removed from games %v", c.ID, id, left)
		}
	}
}

func isHostHello(p blaze.Packet) bool {
	switch {
	case p.Component == Util && p.Command == 7:
		return bytes.Contains(p.Payload, []byte("warsaw server"))
	case p.Component == Authentication && p.Command == 0x28:
		return true
	}
	return false
}

func (c *Conn) becomeHost() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.host != nil {
		return
	}
	c.host = &hostSession{}
	if c.pushToken != 0 {
		ClearPushNotification(c.pushToken)
		c.pushToken = 0
	}
	logger.Info("CONN %d: %s identified as a GAME HOST (warsaw server)", c.ID, c.Remote)
}

func (c *Conn) ensureClientPush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Push == nil {
		return
	}
	if c.pushToken == 0 || !pushIsCurrent(c.pushToken) {
		c.pushToken = SetPushNotification(c.Push)
	}
}

func pushIsCurrent(token uint64) bool {
	pushMu.Lock()
	defer pushMu.Unlock()
	return token == pushToken && PushNotification != nil
}
