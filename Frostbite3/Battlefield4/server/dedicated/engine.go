package dedicated

import (
	"bf4/logger"

	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// GameEngine is the part of a dedicated server that speaks Frostbite's game
// protocol with the PS3s and runs the match.
//
// The dedicated package handles everything around it (Blaze login, game registration, the
// game port, player bookkeeping, reporting connections to Blaze). An engine
// gets every UDP packet that arrives on the game port and returns:
//
//   - replies: packets to send back to the sender
//   - connected: true once the sender has completed its connection, so the
//     server reports the player to Blaze as connected (updateMeshConnection)
//
// No public implementation of BF4's game protocol exists. The first packet a
// joining PS3 sends is a 24-byte connection request: byte 0 is a counter
// (+3 per retry), byte 1 is 0x80, bytes 2..23 are encrypted. A real engine

type GameEngine interface {
	Name() string
	HandlePacket(from *net.UDPAddr, player string, data []byte) (replies [][]byte, connected bool)
	Capture(proto, from string, data []byte)
}

type CaptureEngine struct {
	mu   sync.Mutex
	file *os.File
	seen map[string]int
}

func NewCaptureEngine(dir string) *CaptureEngine {
	e := &CaptureEngine{seen: map[string]int{}}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Warn("DEDICATED: captures: %v (not capturing)", err)
		return e
	}
	path := filepath.Join(dir, "dedicated_"+time.Now().Format("20060102_150405")+".tsv")
	f, err := os.Create(path)
	if err != nil {
		logger.Warn("DEDICATED: captures: %v (not capturing)", err)
		return e
	}
	fmt.Fprintln(f, "time\tproto\tfrom\tplayer\tlen\thex")
	e.file = f
	logger.Info("DEDICATED: capturing game-port traffic to %s", path)
	return e
}

func (e *CaptureEngine) Name() string { return "capture only (no game protocol)" }

func (e *CaptureEngine) HandlePacket(from *net.UDPAddr, player string, data []byte) ([][]byte, bool) {
	e.mu.Lock()
	e.seen[from.String()]++
	n := e.seen[from.String()]
	e.mu.Unlock()

	if len(data) == 24 && data[1] == 0x80 {
		if n == 1 || n%5 == 0 {
			logger.Info("DEDICATED:   %s (%s) connection request #%d: counter=%d flag=0x80, 22 encrypted bytes", from, player, n, data[0])
		}
	} else if n <= 20 {
		logger.Info("DEDICATED:   %s (%s) packet #%d, %d bytes: %s", from, player, n, len(data), short(data, 32))
	}

	e.write("UDP", from.String(), player, data)
	return nil, false
}

func (e *CaptureEngine) Capture(proto, from string, data []byte) {
	e.write(proto, from, "", data)
}

func (e *CaptureEngine) write(proto, from, player string, data []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.file == nil {
		return
	}
	fmt.Fprintf(e.file, "%s\t%s\t%s\t%s\t%d\t%s\n",
		time.Now().Format("15:04:05.000000"), proto, from, player, len(data), hex.EncodeToString(data))
}

func short(b []byte, n int) string {
	if len(b) > n {
		return hex.EncodeToString(b[:n]) + "..."
	}
	return hex.EncodeToString(b)
}
