// Package runtimebundle installs the release's native runtime without downloads.
package runtimebundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/xuges/remote-computer-use/internal/host"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var errBundle = errors.New("invalid native runtime bundle or cache")

type file struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type metadata struct {
	Format   int    `json:"format"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	MinGlibc string `json:"min_glibc"`
	Files    []file `json:"files"`
}

func Embedded() bool { return len(archive) != 0 }

// Directory retains the explicit development override even in embedded builds.
func Directory() (string, error) {
	if dir := os.Getenv("REMOTE_COMPUTER_LIB_DIR"); dir != "" {
		if !filepath.IsAbs(dir) {
			return "", errBundle
		}
		return dir, nil
	}
	if !Embedded() {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		return filepath.Dir(exe), nil
	}
	base := os.Getenv("REMOTE_COMPUTER_CACHE_DIR")
	if base == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(cache, "remote-computer-use", "native")
	}
	if !filepath.IsAbs(base) {
		return "", errBundle
	}
	return unpack(base, manifest, archive)
}

func parse(data []byte) (metadata, error) {
	var m metadata
	if err := json.Unmarshal(data, &m); err != nil {
		return m, errBundle
	}
	if m.Format != 1 || m.OS != runtime.GOOS || m.Arch != runtime.GOARCH || len(m.Files) == 0 || len(m.Files) > 256 {
		return m, errBundle
	}
	seen := map[string]bool{}
	var total int64
	for _, f := range m.Files {
		hash, err := hex.DecodeString(f.SHA256)
		if f.Name == "" || f.Name == "." || f.Name == ".." || strings.TrimRight(f.Name, " .") != f.Name || !safeLibraryName(f.Name) || strings.ContainsAny(f.Name, "/\\:\x00") || seen[strings.ToLower(f.Name)] || f.Size <= 0 || f.Size > 128<<20 || err != nil || len(hash) != sha256.Size {
			return m, errBundle
		}
		seen[strings.ToLower(f.Name)] = true
		total += f.Size
	}
	if total > 256<<20 {
		return m, errBundle
	}
	return m, nil
}

func verify(dir string, m metadata) error {
	if err := host.CheckPrivate(dir, true); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != len(m.Files) {
		return errBundle
	}
	for _, f := range m.Files {
		path := filepath.Join(dir, f.Name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || host.CheckPrivate(path, false) != nil || info.Size() != f.Size {
			return errBundle
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, in)
		closeErr := in.Close()
		if err != nil || closeErr != nil || hex.EncodeToString(hash.Sum(nil)) != f.SHA256 {
			return errBundle
		}
	}
	return nil
}

func unpack(base string, manifest, archive []byte) (string, error) {
	m, err := parse(manifest)
	if err != nil {
		return "", err
	}
	if err = host.MkdirPrivate(base); err != nil {
		return "", err
	}
	if err = host.CheckPrivate(base, true); err != nil {
		return "", err
	}
	id := fmt.Sprintf("%x", sha256.Sum256(manifest))
	dir := filepath.Join(base, id)
	if _, err = os.Lstat(dir); err == nil {
		return dir, verify(dir, m)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	// Publish only complete, verified directories. Concurrent first starts may
	// unpack separately; the first rename wins and the others verify that result.
	tmp, err := os.MkdirTemp(base, ".unpack-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err = host.Protect(tmp, true); err != nil {
		return "", err
	}
	zipped, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", err
	}
	defer zipped.Close()
	reader := tar.NewReader(zipped)
	expected := make(map[string]file, len(m.Files))
	for _, f := range m.Files {
		expected[f.Name] = f
	}
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		f, ok := expected[h.Name]
		if !ok || h.Typeflag != tar.TypeReg || h.Size != f.Size {
			return "", errBundle
		}
		delete(expected, h.Name)
		out, err := os.OpenFile(filepath.Join(tmp, h.Name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(out, reader)
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil {
			return "", errBundle
		}
		if err = host.Protect(filepath.Join(tmp, h.Name), false); err != nil {
			return "", err
		}
	}
	if len(expected) != 0 {
		return "", errBundle
	}
	// Drain gzip too, checking its checksum rather than just the tar end marker.
	if _, err = io.Copy(io.Discard, zipped); err != nil {
		return "", err
	}
	if err = verify(tmp, m); err != nil {
		return "", err
	}
	if err = os.Rename(tmp, dir); err != nil {
		if err = verify(dir, m); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// Library basenames are portable across filesystems, including Windows device
// names and alternate data streams. The manifest never carries arbitrary paths.
func safeLibraryName(name string) bool {
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._+-", r)) {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	return true
}
