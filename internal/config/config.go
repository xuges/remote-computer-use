package config

import (
	"github.com/BurntSushi/toml"
	"github.com/xuges/remote-computer-use/internal/core"
	"github.com/xuges/remote-computer-use/internal/host"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,39}$`)

func ValidName(s string) bool { return namePattern.MatchString(s) }

// Private is intentionally never used in a public response or log.
type Private struct {
	Protocol         string `toml:"protocol"`
	Host             string `toml:"host"`
	Port             int    `toml:"port"`
	Username         string `toml:"username"`
	Password         string `toml:"password"`
	PasswordEnv      string `toml:"password_env"`
	Domain           string `toml:"domain"`
	Width            int    `toml:"width"`
	Height           int    `toml:"height"`
	Fingerprint      string `toml:"certificate_sha256"`
	TLSVerify        *bool  `toml:"tls_verify"`
	AllowUnencrypted bool   `toml:"allow_unencrypted"`
}

// Omitted tls_verify retains certificate verification, including for callers
// constructing Private directly rather than decoding TOML.
func (c Private) SkipTLSVerification() bool { return c.TLSVerify != nil && !*c.TLSVerify }

type File struct {
	Version     int                `toml:"version"`
	Connections map[string]Private `toml:"connections"`
}

func DefaultPath() string {
	if p := os.Getenv("REMOTE_COMPUTER_CONFIG"); p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".remote-computer-use", "config.toml")
}

func Load(path string) (File, error) {
	var f File
	if err := host.CheckPrivate(path, false); err != nil {
		return f, core.InvalidConfig
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 1<<20 {
		return f, core.InvalidConfig
	}
	m, err := toml.Decode(string(b), &f)
	if err != nil || len(m.Undecoded()) != 0 || f.Version != 1 {
		return File{}, core.InvalidConfig
	}
	for n, c := range f.Connections {
		if !ValidName(n) || c.Host == "" || strings.ContainsAny(c.Host+c.Username+c.Password+c.Domain, "\x00") {
			return File{}, core.InvalidConfig
		}
		if c.Password != "" && c.PasswordEnv != "" {
			return File{}, core.InvalidConfig
		}
		if c.Protocol != "vnc" && c.Protocol != "rdp" {
			return File{}, core.InvalidConfig
		}
		if c.Port == 0 {
			if c.Protocol == "vnc" {
				c.Port = 5900
			} else {
				c.Port = 3389
			}
		}
		if c.Port < 1 || c.Port > 65535 {
			return File{}, core.InvalidConfig
		}
		if c.Width == 0 {
			c.Width = 1600
		}
		if c.Height == 0 {
			c.Height = 900
		}
		if c.Width < 64 || c.Height < 64 || c.Width > 8192 || c.Height > 8192 || c.Width*c.Height > 16<<20 {
			return File{}, core.InvalidConfig
		}
		c.Fingerprint = strings.ToLower(strings.ReplaceAll(c.Fingerprint, ":", ""))
		if c.Fingerprint != "" && !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(c.Fingerprint) {
			return File{}, core.InvalidConfig
		}
		if (c.TLSVerify != nil && c.Protocol != "rdp") || (c.SkipTLSVerification() && c.Fingerprint != "") {
			return File{}, core.InvalidConfig
		}
		f.Connections[n] = c
	}
	return f, nil
}

func (c Private) Resolve() (Private, error) {
	if c.PasswordEnv != "" {
		v, ok := os.LookupEnv(c.PasswordEnv)
		if !ok || strings.ContainsRune(v, 0) {
			return Private{}, core.InvalidConfig
		}
		c.Password, c.PasswordEnv = v, ""
	}
	return c, nil
}
