package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorDoesNotReadPrivateConfiguration(t *testing.T) {
	root := t.TempDir()
	secret := "PRIVATE-CREDENTIAL-MARKER"
	config := filepath.Join(root, "config.toml")
	if err := os.WriteFile(config, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REMOTE_COMPUTER_CONFIG", config)
	t.Setenv("REMOTE_COMPUTER_LIB_DIR", filepath.Join(root, secret))
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = w
	code := Ctl([]string{"doctor", "--json"})
	os.Stdout = original
	w.Close()
	data, _ := io.ReadAll(r)
	r.Close()
	var result struct {
		OK      bool   `json:"ok"`
		Runtime string `json:"runtime"`
		Checks  []struct {
			Error string `json:"error"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if code != 1 || result.OK || result.Runtime != "override" || len(result.Checks) != 2 {
		t.Fatalf("unexpected doctor report: %s", data)
	}
	for _, check := range result.Checks {
		if check.Error != "BACKEND_UNAVAILABLE" {
			t.Fatalf("unexpected check: %s", data)
		}
	}
	if strings.Contains(string(data), secret) || strings.Contains(string(data), root) {
		t.Fatal("doctor exposed private data")
	}
}
