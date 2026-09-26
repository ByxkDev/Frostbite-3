package logger

import (
	"fmt"
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

func Debug(format string, args ...any) {
	mu.Lock()
	enabled := debug
	mu.Unlock()

	if enabled {
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
	mu.Lock()
	enabled := trace
	mu.Unlock()

	if enabled {
		write(LevelTrace, format, args...)
	}
}

func Hex(level Level, label string, data []byte) {
	if level == LevelDebug {
		mu.Lock()
		enabled := debug
		mu.Unlock()

		if !enabled {
			return
		}
	}

	if level == LevelTrace {
		mu.Lock()
		enabled := trace
		mu.Unlock()

		if !enabled {
			return
		}
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

func Packet(direction string, component, command uint16, packetType uint8, messageID uint32, payload []byte) {
	Debug("%s Component=%d Command=%d Type=0x%02X MessageId=%d Payload=%d bytes", direction, component, command, packetType, messageID, len(payload),)
	Hex(LevelDebug, direction+" PAYLOAD", payload)
}

func write(level Level, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05.000")

	mu.Lock()
	defer mu.Unlock()

	fmt.Printf("[%s] [%s] %s\n", timestamp, level, message)
}