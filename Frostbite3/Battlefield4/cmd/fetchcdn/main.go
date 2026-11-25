package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const cdnHost = "eaassets-a.akamaihd.net"

var lists = []string{
	"/dice-commerce/battlefield4/editorial/prod/20151027-01/store/gen3/ps3/storeitems_en.json",
	"/dice-commerce/battlefield4/editorial/prod/20150722-01/premium/ps3/premium_en.json",
}

var imageFields = []string{
	"mainImageUrl", "backgroundImageUrl",
	"thumbImageUrl", "featuredImageUrl", "timelineImageUrl", "detailImageUrl",
}

func main() {
	dir := flag.String("dir", "data/cdn", "folder the files are saved under (by URL path)")
	dns := flag.String("dns", "1.1.1.1:53", "DNS server used to find the real CDN; empty = system DNS")
	force := flag.Bool("force", false, "download again even if the file exists")
	flag.Parse()

	client := newClient(*dns)

	var paths []string
	seen := map[string]bool{}
	for _, l := range lists {
		items, err := loadItems(filepath.Join(*dir, filepath.FromSlash(strings.TrimPrefix(l, "/"))))
		if err != nil {
			fmt.Println("skip list", l, "-", err)
			continue
		}
		for _, p := range imagePaths(l, items) {
			if !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	fmt.Printf("%d images to fetch\n", len(paths))

	var ok, skipped, failed int
	for _, p := range paths {
		dst := filepath.Join(*dir, filepath.FromSlash(strings.TrimPrefix(p, "/")))
		url := "http://" + cdnHost + p
		if !*force {
			if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
				fmt.Println("skip (exists)", dst)
				skipped++
				continue
			}
		}
		n, err := download(client, url, dst)
		if err != nil {
			fmt.Println("FAIL", url, "-", err)
			failed++
			continue
		}
		fmt.Printf("ok   %s (%d bytes)\n", dst, n)
		ok++
	}
	fmt.Printf("\ndone: %d downloaded, %d skipped, %d failed\n", ok, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func newClient(dnsServer string) *http.Client {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if dnsServer != "" {
		dialer.Resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, "udp", dnsServer)
			},
		}
	}
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext, Proxy: http.ProxyFromEnvironment},
	}
}

func loadItems(file string) ([]map[string]any, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var items []map[string]any
	return items, json.Unmarshal(b, &items)
}

func imagePaths(listURL string, items []map[string]any) []string {
	base := path.Dir(listURL)
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		for _, f := range imageFields {
			u, _ := it[f].(string)
			if u == "" {
				continue
			}
			p := path.Clean(path.Join(base, u))
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func download(c *http.Client, url, dst string) (int64, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "ProtoHttp 1.3/DS 13.3.1.2.1 (PlayStation3)")
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	return n, os.Rename(tmp, dst)
}
