package app

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/xuges/remote-computer-use/internal/core"
)

func TestActionRejectsUnrelatedFlagsBeforeConnecting(t *testing.T) {
	for _, args := range [][]string{
		{"click", "--x", "10", "--y", "20", "--text", "ignored"},
		{"screenshot", "--output", "ignored.png", "--button", "right"},
		{"type", "--text", "hello", "--count", "2"},
		{"key", "CTRL+A", "--duration", "1s"},
	} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		original := os.Stdout
		os.Stdout = w
		code := Use(append([]string{"--conn", "test", "--json"}, args...))
		os.Stdout = original
		w.Close()
		data, _ := io.ReadAll(r)
		r.Close()
		var result core.Result
		if json.Unmarshal(data, &result) != nil || code != 1 || result.Error != core.InvalidArgs {
			t.Fatalf("unexpected result for %v: %s", args, data)
		}
	}
}
