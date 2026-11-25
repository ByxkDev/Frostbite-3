package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bf4/handshake"
)

func main() {
	file := flag.String("f", "data/captures/hsdiff.jsonl", "handshake lab records")
	all := flag.Bool("all", false, "print every individual finding")
	importTSV := flag.Bool("import-tsv", false, "import the TSV capture files given as arguments into -f first")
	flag.Parse()

	st, err := handshake.OpenStore(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hsdiff:", err)
		os.Exit(1)
	}

	if *importTSV {
		n := 0
		for _, pattern := range flag.Args() {
			matches, _ := filepath.Glob(pattern)
			for _, m := range matches {
				n += importFile(st, m)
			}
		}
		fmt.Printf("imported %d new connection requests\n", n)
	}

	rep := handshake.Analyze(st.Records())
	if *all {
		for _, f := range rep.Findings {
			fmt.Println(f)
		}
		fmt.Println()
	}
	for _, line := range rep.Lines() {
		fmt.Println(line)
	}
}

func importFile(st *handshake.Store, path string) int {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hsdiff:", err)
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		cols := strings.Split(sc.Text(), "\t")
		if len(cols) < 6 || cols[1] != "UDP" {
			continue
		}
		pkt, err := hex.DecodeString(cols[5])
		if err != nil {
			continue
		}
		ctx := map[string]string{"name": cols[3]}
		if host, port, err := net.SplitHostPort(cols[2]); err == nil {
			ctx["client.ip"] = host
			if _, err := strconv.Atoi(port); err == nil {
				ctx["client.port"] = port
			}
		}
		if rec, ok := handshake.NewRecord(cols[2], pkt, ctx); ok {
			if _, isNew, _ := st.Add(rec); isNew {
				n++
			}
		}
	}
	return n
}
