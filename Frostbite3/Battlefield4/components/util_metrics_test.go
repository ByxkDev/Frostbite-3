package components

import (
	"testing"

	"bf4/blaze"
)

func TestUtilSetClientMetricsReplies(t *testing.T) {
	p := blaze.Packet{Component: 9, Command: 22, MessageId: 28, Payload: []byte{0xD6, 0x29, 0xAC, 0x00, 0x02}}
	out := HandleUtil(p)
	if len(out) < 12 {
		t.Fatalf("no reply: %x", out)
	}
	r := blaze.Parse(out)
	if r.Component != 9 || r.Command != 22 || r.MessageId != 28 {
		t.Fatalf("reply header %+v", r)
	}
}
