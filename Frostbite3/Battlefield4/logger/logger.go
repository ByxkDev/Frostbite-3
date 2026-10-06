package logger

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Level string

const (
	LevelDebug Level = "DEBUG"
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
	LevelTrace Level = "TRACE"
)

var (
	mu    sync.Mutex
	debug = true
	trace = true
)

func Init(enableDebug, enableTrace bool) {
	mu.Lock()
	debug = enableDebug
	trace = enableTrace
	mu.Unlock()

	Info("Logger initialized")
	Info("Debug logging: %t", enableDebug)
	Info("Trace logging: %t", enableTrace)
}

func Close() {}

func SetDebug(enabled bool) {
	mu.Lock()
	debug = enabled
	mu.Unlock()
}

func SetTrace(enabled bool) {
	mu.Lock()
	trace = enabled
	mu.Unlock()
}

func DebugEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return debug
}

func TraceEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return trace
}

func levelEnabled(level Level) bool {
	switch level {
	case LevelDebug:
		return DebugEnabled()
	case LevelTrace:
		return TraceEnabled()
	default:
		return true
	}
}

func Debug(format string, args ...any) {
	if DebugEnabled() {
		write(LevelDebug, format, args...)
	}
}

func Info(format string, args ...any) {
	write(LevelInfo, format, args...)
}

func Warn(format string, args ...any) {
	write(LevelWarn, format, args...)
}

func Error(format string, args ...any) {
	write(LevelError, format, args...)
}

func Trace(format string, args ...any) {
	if TraceEnabled() {
		write(LevelTrace, format, args...)
	}
}

func Hex(level Level, label string, data []byte) {
	if !levelEnabled(level) {
		return
	}

	write(level, "%s (%d bytes): % X", label, len(data), data)
}

func Request(data []byte) {
	Trace("REQUEST (%d bytes)", len(data))
	Hex(LevelTrace, "REQUEST HEX", data)
}

func Response(data []byte) {
	Trace("RESPONSE (%d bytes)", len(data))
	Hex(LevelTrace, "RESPONSE HEX", data)
}

func Packet(direction string, component, command uint16, packetType uint16, messageID uint32, payload []byte) {
	Debug("%s Component=%d Command=%d Type=0x%04X MessageId=%d Payload=%d bytes", direction, component, command, packetType, messageID, len(payload))
	Hex(LevelDebug, direction+" PAYLOAD", payload)
}

func Section(level Level, title string) {
	if !levelEnabled(level) {
		return
	}

	write(level, "---------- %s ----------", title)
}

func KV(key string, value any) {
	Debug("  %-22s = %v", key, value)
}

func KVq(key string, value any) {
	Debug("  %-22s = %q", key, value)
}

func Struct(label string, v any) {
	Debug("%s: %+v", label, v)
}

func HexDump(level Level, label string, data []byte) {
	if !levelEnabled(level) {
		return
	}

	write(level, "%s (%d bytes)", label, len(data))

	if len(data) == 0 {
		return
	}

	for off := 0; off < len(data); off += 16 {
		end := off + 16
		if end > len(data) {
			end = len(data)
		}

		chunk := data[off:end]

		var hex strings.Builder
		for i := 0; i < 16; i++ {
			if i < len(chunk) {
				fmt.Fprintf(&hex, "%02X ", chunk[i])
			} else {
				hex.WriteString("   ")
			}
			if i == 7 {
				hex.WriteString(" ")
			}
		}

		var ascii strings.Builder
		for _, b := range chunk {
			if b >= 32 && b <= 126 {
				ascii.WriteByte(b)
			} else {
				ascii.WriteByte('.')
			}
		}

		write(level, "  %04X  %s |%s|", off, hex.String(), ascii.String())
	}
}

func Analyze(label string, data []byte) {
	if !DebugEnabled() {
		return
	}

	printable := 0
	zeros := 0
	var nullPos []int

	for i, b := range data {
		if b >= 32 && b <= 126 {
			printable++
		}
		if b == 0 {
			zeros++
			if len(nullPos) < 16 {
				nullPos = append(nullPos, i)
			}
		}
	}

	write(LevelDebug, "ANALYZE %s: size=%d printable=%d zeros=%d other=%d", label, len(data), printable, zeros, len(data)-printable-zeros)

	if len(data) == 0 {
		return
	}

	head := data
	if len(head) > 8 {
		head = head[:8]
	}

	tail := data
	if len(tail) > 8 {
		tail = tail[len(tail)-8:]
	}

	write(LevelDebug, "ANALYZE %s: first=% X last=% X", label, head, tail)

	if len(nullPos) > 0 {
		write(LevelDebug, "ANALYZE %s: null positions (first %d)=%v", label, len(nullPos), nullPos)
	}

	for i, s := range PrintableStrings(data, 4) {
		write(LevelDebug, "ANALYZE %s: string[%d]=%q", label, i, s)
	}
}

func PrintableStrings(data []byte, minLen int) []string {
	var out []string
	var cur []byte

	flush := func() {
		if len(cur) >= minLen {
			out = append(out, string(cur))
		}
		cur = cur[:0]
	}

	for _, b := range data {
		if b >= 32 && b <= 126 {
			cur = append(cur, b)
		} else {
			flush()
		}
	}
	flush()

	return out
}

func Equal(label string, a, b []byte) bool {
	if bytes.Equal(a, b) {
		Debug("%s: MATCH (%d bytes)", label, len(a))
		return true
	}

	n := len(a)
	if len(b) < n {
		n = len(b)
	}

	diff := n
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			diff = i
			break
		}
	}

	Warn("%s: MISMATCH lenA=%d lenB=%d firstDiff=%d", label, len(a), len(b), diff)
	return false
}

func Mask(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}

	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

func Timed(name string) func() {
	start := time.Now()
	Trace("%s: enter", name)

	return func() {
		Debug("%s: done in %s", name, time.Since(start))
	}
}

func write(level Level, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05.000")

	mu.Lock()
	fmt.Printf("[%s] [%s] %s\n", timestamp, level, message)
	mu.Unlock()
}
