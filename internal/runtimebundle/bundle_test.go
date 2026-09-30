package runtimebundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func fixture(t *testing.T, name string, kind byte) ([]byte, []byte) {
	t.Helper()
	data := []byte("native-library-fixture")
	m, err := json.Marshal(metadata{Format: 1, OS: runtime.GOOS, Arch: runtime.GOARCH, Files: []file{{Name: name, Size: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}}})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	w := tar.NewWriter(z)
	h := &tar.Header{Name: name, Typeflag: kind, Size: int64(len(data)), Mode: 0600}
	if kind == tar.TypeSymlink {
		h.Linkname = "/tmp/outside"
		h.Size = 0
	}
	if err := w.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if kind == tar.TypeReg {
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return m, buf.Bytes()
}

func TestConcurrentExtractionAndReuse(t *testing.T) {
	m, a := fixture(t, "libtest.so", tar.TypeReg)
	base := filepath.Join(t.TempDir(), "cache")
	var wg sync.WaitGroup
	dirs := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, e := unpack(base, m, a)
			if e != nil {
				t.Error(e)
			}
			dirs <- d
		}()
	}
	wg.Wait()
	close(dirs)
	var want string
	for d := range dirs {
		if want == "" {
			want = d
		}
		if d != want {
			t.Fatal("different cache locations")
		}
	}
	info, err := os.Stat(filepath.Join(want, "libtest.so"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("unsafe file mode")
	}
	// Cache hit does not need to decompress the archive again.
	if d, e := unpack(base, m, nil); e != nil || d != want {
		t.Fatalf("reuse: %q %v", d, e)
	}
	entries, _ := os.ReadDir(base)
	if len(entries) != 1 {
		t.Fatalf("temporary directories leaked: %v", entries)
	}
}

func TestRejectUnsafeOrCorruptCache(t *testing.T) {
	for _, kind := range []string{"content", "symlink", "directory-mode", "file-mode"} {
		t.Run(kind, func(t *testing.T) {
			m, a := fixture(t, "libtest.so", tar.TypeReg)
			base := filepath.Join(t.TempDir(), "cache")
			d, e := unpack(base, m, a)
			if e != nil {
				t.Fatal(e)
			}
			p := filepath.Join(d, "libtest.so")
			switch kind {
			case "content":
				e = os.WriteFile(p, []byte("broken"), 0600)
			case "symlink":
				if e = os.Remove(p); e == nil {
					e = os.Symlink("/tmp/missing", p)
				}
			case "directory-mode":
				e = os.Chmod(d, 0755)
			case "file-mode":
				e = os.Chmod(p, 0666)
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = unpack(base, m, a); e == nil {
				t.Fatal("accepted unsafe cache")
			}
		})
	}
}

func TestRejectUnsafeArchiveAndPlatform(t *testing.T) {
	for _, name := range []string{"../outside", "/outside", "nested/file", ".."} {
		m, a := fixture(t, name, tar.TypeReg)
		if _, e := unpack(filepath.Join(t.TempDir(), "cache"), m, a); e == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	m, a := fixture(t, "libtest.so", tar.TypeSymlink)
	if _, e := unpack(filepath.Join(t.TempDir(), "cache"), m, a); e == nil {
		t.Fatal("accepted archive symlink")
	}
	m, a = fixture(t, "libtest.so", tar.TypeReg)
	var meta metadata
	json.Unmarshal(m, &meta)
	meta.Arch = "wrong-architecture"
	bad, _ := json.Marshal(meta)
	if _, e := unpack(filepath.Join(t.TempDir(), "cache"), bad, a); e == nil {
		t.Fatal("accepted wrong architecture")
	}
	a[len(a)-1] ^= 1
	base := filepath.Join(t.TempDir(), "cache")
	if _, e := unpack(base, m, a); e == nil {
		t.Fatal("accepted bad gzip checksum")
	}
	entries, _ := os.ReadDir(base)
	if len(entries) != 0 {
		t.Fatal("published partial bundle")
	}
}

func TestRejectSymlinkCacheRoot(t *testing.T) {
	m, a := fixture(t, "libtest.so", tar.TypeReg)
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if e := os.Mkdir(target, 0700); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(base, "link")
	if e := os.Symlink(target, link); e != nil {
		t.Fatal(e)
	}
	if _, e := unpack(link, m, a); e == nil {
		t.Fatal("followed cache symlink")
	}
}

func TestDirectoryOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REMOTE_COMPUTER_LIB_DIR", dir)
	if got, e := Directory(); e != nil || got != dir {
		t.Fatalf("override: %q %v", got, e)
	}
	t.Setenv("REMOTE_COMPUTER_LIB_DIR", "relative")
	if _, e := Directory(); e == nil {
		t.Fatal("accepted relative override")
	}
}

func TestBundleVersionsDoNotReplaceActiveLibraries(t *testing.T) {
	base := filepath.Join(t.TempDir(), "cache")
	m1, a1 := fixture(t, "libfirst.so", tar.TypeReg)
	m2, a2 := fixture(t, "libsecond.so", tar.TypeReg)
	d1, e := unpack(base, m1, a1)
	if e != nil {
		t.Fatal(e)
	}
	d2, e := unpack(base, m2, a2)
	if e != nil {
		t.Fatal(e)
	}
	if d1 == d2 {
		t.Fatal("versions share cache directory")
	}
	if _, e := os.Stat(filepath.Join(d1, "libfirst.so")); e != nil {
		t.Fatal("old runtime removed", e)
	}
}
