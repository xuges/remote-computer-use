package app

import (
	"bytes"
	"encoding/binary"
	"github.com/xuges/remote-computer-use/internal/core"
	"testing"
)

func TestBinaryPacketRoundtrip(t *testing.T) {
	data := bytes.Repeat([]byte{0, 255, 13, 10, 128}, 250000)
	var wire bytes.Buffer
	if e := writePacket(&wire, core.Result{OK: true, PNG: data}); e != nil {
		t.Fatal(e)
	}
	var got core.Result
	if e := readPacket(&wire, &got); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(data, got.PNG) {
		t.Fatal("binary bytes changed")
	}
}

func TestPacketRejectsOversizedAndTruncatedInput(t *testing.T) {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], maxPacket+1)
	for _, b := range [][]byte{header[:], {0, 0, 0, 5, '{'}} {
		var v core.Result
		if readPacket(bytes.NewReader(b), &v) == nil {
			t.Fatal("invalid packet accepted")
		}
	}
}
