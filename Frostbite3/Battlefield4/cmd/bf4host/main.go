package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"bf4/logger"
	"bf4/server/dedicated"
)

func main() {
	c := dedicated.DefaultConfig()
	flag.StringVar(&c.Blaze, "blaze", c.Blaze, "Blaze game port (host:port) of the emulator")
	flag.StringVar(&c.IP, "ip", c.IP, "IPv4 address players connect to")
	flag.IntVar(&c.Port, "port", c.Port, "UDP/TCP game port players connect to")
	flag.StringVar(&c.Name, "name", c.Name, "server name")
	flag.StringVar(&c.Level, "level", c.Level, "level (LevelName from the level description)")
	flag.StringVar(&c.Mode, "mode", c.Mode, "game mode (ConquestLarge0, ConquestSmall0, Domination0, Elimination0, GunMaster0, Obliteration, RushLarge0, SquadDeathMatch0, SquadObliteration0, TeamDeathMatch0)")
	flag.StringVar(&c.Mod, "mod", c.Mod, "mod / map pack (DEFAULT, XPACK0..XPACK7)")
	flag.IntVar(&c.Players, "players", c.Players, "player slots (max 64)")
	flag.IntVar(&c.Spectators, "spectators", c.Spectators, "spectator slots")
	flag.IntVar(&c.Queue, "queue", c.Queue, "queue capacity")
	flag.StringVar(&c.Mail, "mail", c.Mail, "server account mail")
	flag.StringVar(&c.Password, "password", c.Password, "server account password")
	flag.StringVar(&c.Persona, "persona", c.Persona, "server persona name")
	flag.StringVar(&c.Version, "version", c.Version, "game protocol version (the PS3's GVER)")
	flag.DurationVar(&c.JoinTimeout, "join-timeout", c.JoinTimeout, "how long a joining player may take to connect")
	flag.BoolVar(&c.FakeConnect, "fake-connect", c.FakeConnect, "TESTING ONLY: report players connected on their first packet")
	flag.StringVar(&c.CaptureDir, "captures", c.CaptureDir, "folder for packet captures")
	flag.Parse()

	logger.Init(true, true)
	defer logger.Close()

	ds, err := dedicated.Start(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bf4host:", err)
		os.Exit(1)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	ds.Stop()
}
