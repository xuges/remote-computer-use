package config

import (
	"github.com/xuges/remote-computer-use/internal/core"
	"os"
	"path/filepath"
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
			p := filepath.Join(t.TempDir(), "config.toml")
			if e := os.WriteFile(p, []byte(base+tc.extra), tc.mode); e != nil {
				t.Fatal(e)
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
