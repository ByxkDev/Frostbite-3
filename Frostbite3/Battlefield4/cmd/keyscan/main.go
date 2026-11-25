package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"bf4/server/dedicated"
)

var (
	rePkt  = regexp.MustCompile(`handshake: "([^"]*)" (?:first packet counter=(\d+)|counter \d+ -> (\d+)|new attempt counter=(\d+)).*payload=([0-9A-Fa-f]+)`)
	reUUID = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

type pkt struct {
	who     string
	counter int
	raw     []byte
}

func main() {
	logPath := flag.String("log", "", "dedicated server log")
	uuid := flag.String("uuid", "", "game UUID the PS3 received (default: try every UUID in the log)")
	flag.Parse()
	if *logPath == "" {
		fmt.Fprintln(os.Stderr, "usage: keyscan --log server.log [--uuid UUID]")
		os.Exit(2)
	}
	f, err := os.Open(*logPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	var pkts []pkt
	var uuids []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		for _, u := range reUUID.FindAllString(line, -1) {
			if !seen[u] {
				seen[u] = true
				uuids = append(uuids, u)
			}
		}
		m := rePkt.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		c := -1
		for _, s := range m[2:5] {
			if s != "" {
				c, _ = strconv.Atoi(s)
			}
		}
		body, err := hex.DecodeString(m[5])
		if err != nil || c < 0 {
			continue
		}
		pkts = append(pkts, pkt{who: m[1], counter: c, raw: append([]byte{byte(c), 0x80}, body...)})
	}
	if len(pkts) == 0 {
		fmt.Println("no handshake packets in the log")
		os.Exit(1)
	}
	if *uuid != "" {
		uuids = []string{strings.ToLower(*uuid)}
	}

	for _, u := range uuids {
		key := []byte(dedicated.TunnelKey(u, "", ""))
		if _, ok := dedicated.TunnelOpen(key, pkts[0].raw); !ok {
			continue
		}
		fmt.Printf("key = %q\n\n", key)
		for _, p := range pkts {
			subs, ok := dedicated.TunnelOpen(key, p.raw)
			if !ok {
				fmt.Printf("%-10s c=%-4d does not open (different join?)\n", p.who, p.counter)
				continue
			}
			var parts []string
			for _, s := range subs {
				parts = append(parts, fmt.Sprintf("port%d[%d]=%X", s.Port, len(s.Data), s.Data))
			}
			fmt.Printf("%-10s c=%-4d %s\n", p.who, p.counter, strings.Join(parts, " "))
		}
		return
	}
	fmt.Printf("none of the %d UUIDs in the log opens the packets; pass the one the PS3 got with --uuid\n", len(uuids))
	os.Exit(1)
}
