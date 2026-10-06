package dedicated

import (
	"bf4/logger"

	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"bf4/blaze"
)

const (
	typeRequest      uint16 = 0x0000
	typeReply        uint16 = 0x1000
	typeNotification uint16 = 0x2000
	typeErrorReply   uint16 = 0x3000
	typeMask         uint16 = 0xF000
	flagExtLength    uint16 = 0x0010
)

var (
	tdfFactory = blaze.NewTdfFactory()
	tdfEncoder = tdfFactory.CreateEncoder(true)
	tdfDecoder = tdfFactory.CreateDecoder(true)
)

type packet struct {
	Component uint16
	Command   uint16
	Error     uint16
	Type      uint16
	MsgID     uint16
	Payload   []byte
}

type blazeConn struct {
	conn net.Conn

	writeMu sync.Mutex
	nextID  uint16

	mu      sync.Mutex
	pending map[uint16]chan packet
	notify  chan packet
	done    chan struct{}
	err     error
}

func dialBlaze(addr string) (*blazeConn, error) {
	c, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	logger.Info("DEDICATED: connected to Blaze at %s", addr)
	b := &blazeConn{
		conn:    c,
		pending: map[uint16]chan packet{},
		notify:  make(chan packet, 64),
		done:    make(chan struct{}),
	}
	go b.readLoop()
	return b, nil
}

func (b *blazeConn) Close() { b.conn.Close() }

func (b *blazeConn) Done() <-chan struct{} { return b.done }

func (b *blazeConn) Err() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

func (b *blazeConn) Notifications() <-chan packet { return b.notify }

func (b *blazeConn) Call(component, command uint16, obj interface{}, timeout time.Duration) (packet, error) {
	var payload []byte
	if obj != nil {
		p, err := tdfEncoder.Encode(obj)
		if err != nil {
			return packet{}, fmt.Errorf("encode %d/0x%04X: %w", component, command, err)
		}
		payload = p
	}

	ch := make(chan packet, 1)
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	b.pending[id] = ch
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
	}()

	if err := b.write(component, command, typeRequest, id, payload); err != nil {
		return packet{}, err
	}

	select {
	case r := <-ch:
		if r.Type&typeMask == typeErrorReply {
			return r, fmt.Errorf("Blaze error 0x%04X for %d/0x%04X", r.Error, component, command)
		}
		return r, nil
	case <-b.done:
		return packet{}, fmt.Errorf("connection closed: %v", b.Err())
	case <-time.After(timeout):
		return packet{}, fmt.Errorf("no reply to %d/0x%04X within %s", component, command, timeout)
	}
}

func (b *blazeConn) write(component, command, typ, id uint16, payload []byte) error {
	n := len(payload)
	hdr := 12
	if n > 0xFFFF {
		hdr = 14
		typ |= flagExtLength
	}
	buf := make([]byte, hdr, hdr+n)
	binary.BigEndian.PutUint16(buf[0:], uint16(n))
	binary.BigEndian.PutUint16(buf[2:], component)
	binary.BigEndian.PutUint16(buf[4:], command)
	binary.BigEndian.PutUint16(buf[8:], typ)
	binary.BigEndian.PutUint16(buf[10:], id)
	if hdr == 14 {
		binary.BigEndian.PutUint16(buf[12:], uint16(n>>16))
	}
	buf = append(buf, payload...)

	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	_, err := b.conn.Write(buf)
	return err
}

func (b *blazeConn) readLoop() {
	var err error
	defer func() {
		b.mu.Lock()
		b.err = err
		b.mu.Unlock()
		close(b.done)
		close(b.notify)
	}()

	for {
		var p packet
		p, err = readPacket(b.conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = errors.New("Blaze closed the connection")
			}
			return
		}

		switch p.Type & typeMask {
		case typeReply, typeErrorReply:
			b.mu.Lock()
			ch, ok := b.pending[p.MsgID]
			b.mu.Unlock()
			if ok {
				ch <- p
			}
		case typeNotification:
			select {
			case b.notify <- p:
			default:
				logger.Warn("DEDICATED: notification queue full, dropped %d/0x%04X", p.Component, p.Command)
			}
		}
	}
}

func readPacket(r io.Reader) (packet, error) {
	hdr := make([]byte, 12)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return packet{}, err
	}
	p := packet{
		Component: binary.BigEndian.Uint16(hdr[2:]),
		Command:   binary.BigEndian.Uint16(hdr[4:]),
		Error:     binary.BigEndian.Uint16(hdr[6:]),
		Type:      binary.BigEndian.Uint16(hdr[8:]),
		MsgID:     binary.BigEndian.Uint16(hdr[10:]),
	}
	n := int(binary.BigEndian.Uint16(hdr[0:]))
	if p.Type&flagExtLength != 0 {
		ext := make([]byte, 2)
		if _, err := io.ReadFull(r, ext); err != nil {
			return packet{}, err
		}
		n |= int(binary.BigEndian.Uint16(ext)) << 16
	}
	p.Payload = make([]byte, n)
	if _, err := io.ReadFull(r, p.Payload); err != nil {
		return packet{}, err
	}
	return p, nil
}
