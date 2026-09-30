package config

import (
	"encoding/json"
	"github.com/xuges/remote-computer-use/internal/core"
	"github.com/xuges/remote-computer-use/internal/host"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigRejectsAmbiguityAndProtectsSecrets(t *testing.T) {
	base := "version = 1\n[connections.test]\nprotocol = 'vnc'\nhost = 'PRIVATE-HOST'\npassword = 'PRIVATE-PASSWORD'\n"
	for _, tc := range []struct {
		name, extra string
		mode        os.FileMode
		valid       bool
	}{
		{"valid", "", 0600, true}, {"unknown", "typo = 1\n", 0600, false}, {"world-readable", "", 0644, false}, {"conflicting credentials", "password_env = 'MY_PASS'\n", 0600, false}, {"bad port", "port = 99999\n", 0600, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tc.name == "world-readable" {
				t.Skip("POSIX mode test; Windows ACL tested separately")
			}
			p := filepath.Join(t.TempDir(), "config.toml")
			if e := os.WriteFile(p, []byte(base+tc.extra), tc.mode); e != nil {
				t.Fatal(e)
			}
			if runtime.GOOS == "windows" {
				if err := host.Protect(p, false); err != nil {
					t.Fatal(err)
				}
			}
			f, e := Load(p)
			if tc.valid {
				if e != nil || f.Connections["test"].Port != 5900 {
					t.Fatalf("valid config failed: %v", e)
				}
			} else if e != core.InvalidConfig {
				t.Fatalf("expected safe error, got %v", e)
			}
		})
	}
}

func TestTLSVerificationPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, extra string
		valid, skip           bool
	}{
		{"default verifies", "rdp", "", true, false},
		{"explicit verification", "rdp", "tls_verify = true\n", true, false},
		{"explicit skip", "rdp", "tls_verify = false\n", true, true},
		{"pin with verification", "rdp", "tls_verify = true\ncertificate_sha256 = '" + strings.Repeat("a", 64) + "'\n", true, false},
		{"conflicting pin", "rdp", "tls_verify = false\ncertificate_sha256 = '" + strings.Repeat("a", 64) + "'\n", false, false},
		{"wrong type", "rdp", "tls_verify = 'false'\n", false, false},
		{"VNC does not support this option", "vnc", "tls_verify = false\n", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.toml")
			body := "version = 1\n[connections.test]\nprotocol = '" + tc.protocol + "'\nhost = 'test.invalid'\n" + tc.extra
			if err := os.WriteFile(p, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS == "windows" {
				if err := host.Protect(p, false); err != nil {
					t.Fatal(err)
				}
			}
			f, err := Load(p)
			if !tc.valid {
				if err != core.InvalidConfig {
					t.Fatal("ambiguous certificate policy accepted")
				}
				return
			}
			if err != nil || f.Connections["test"].SkipTLSVerification() != tc.skip {
				t.Fatal("incorrect certificate policy")
			}
			// Private configuration passes through JSON when starting the worker.
			data, err := json.Marshal(f.Connections["test"])
			var worker Private
			if err != nil || json.Unmarshal(data, &worker) != nil || worker.SkipTLSVerification() != tc.skip {
				t.Fatal("certificate policy lost during worker startup")
			}
		})
	}
}

func TestResolveOnlyRequestedCredentials(t *testing.T) {
	t.Setenv("RCU_TEST_PASS", "test-value")
	c, e := (Private{PasswordEnv: "RCU_TEST_PASS"}).Resolve()
	if e != nil || c.Password != "test-value" || c.PasswordEnv != "" {
		t.Fatal("environment resolution failed")
	}
	if _, e = (Private{PasswordEnv: "RCU_NONEXISTENT"}).Resolve(); e != core.InvalidConfig {
		t.Fatal("missing credential accepted")
	}
}
