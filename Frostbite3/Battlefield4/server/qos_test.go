package server

import (
	"net"
	"strings"
	"testing"
)

func TestQosReply(t *testing.T) {
	peer := &net.UDPAddr{IP: net.IPv4(77, 168, 97, 16), Port: 3659}
	latency := []byte{0, 0, 0, 2, 0, 0, 0, 1, 0xa6, 0xea, 0xd5, 0x19, 0, 0, 0, 1, 0x12, 0x34, 0x56, 0x78}
	for range 3 {
		r := QosReply(latency, peer)
		if len(r) != 30 {
			t.Fatalf("len %d", len(r))
		}
		for i := range 20 {
			if r[i] != latency[i] {
				t.Fatalf("latency reply byte %d not echoed", i)
			}
		}
		if r[20] != 77 || r[23] != 16 || int(r[24])<<8|int(r[25]) != 3659 || r[26]|r[27]|r[28]|r[29] != 0 {
			t.Fatalf("address/len wrong: %x", r[20:])
		}
	}
	bandwidth := []byte{0, 0, 0, 7, 0, 0, 3, 0xe9, 0, 0, 0, 0, 0, 0, 0, 3, 0x12, 0x34, 0x56, 0x78}
	if r := QosReply(bandwidth, peer); r[15] != 1 {
		t.Fatalf("first bandwidth reply count %d", r[15])
	}
	if r := QosReply(bandwidth, peer); r[15] != 2 {
		t.Fatalf("second bandwidth reply count %d", r[15])
	}
	if QosReply(make([]byte, 20), peer) != nil {
		t.Fatal("zero request id must be ignored")
	}
	if QosReply(latency[:10], peer) != nil {
		t.Fatal("short packet must be ignored")
	}
}

func TestQosHTTP(t *testing.T) {
	from := &net.TCPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 5}
	r := string(QosHTTP([]byte("GET /qos/qos?vers=1&qtyp=1&prpt=3659 HTTP/1.1\r\nHost: x\r\n\r\n"), from))
	for _, s := range []string{"200 OK", "<numprobes>10</numprobes>", "<qosport>17502</qosport>", "<requestid>"} {
		if !strings.Contains(r, s) {
			t.Fatalf("missing %q in %q", s, r)
		}
	}
	r = string(QosHTTP([]byte("GET /qos/firewall?vers=1&nint=1&inip=1&inpt=2 HTTP/1.1\r\n\r\n"), from))
	if !strings.Contains(r, "<numinterfaces>1</numinterfaces>") || !strings.Contains(r, "<ips><ips>2549368898</ips></ips>") {
		t.Fatalf("firewall: %q", r)
	}
	r = string(QosHTTP([]byte("GET /qos/firetype?vers=1&rqid=1&rqsc=2 HTTP/1.1\r\n\r\n"), from))
	if !strings.Contains(r, "<firetype><firetype>0</firetype></firetype>") {
		t.Fatalf("firetype: %q", r)
	}
	if QosHTTP([]byte("POST / HTTP/1.1\r\n\r\n"), from) != nil {
		t.Fatal("non-qos")
	}
}
