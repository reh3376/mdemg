// ADAPTER-SWAP-STANDARDIZE-001 unit tests
package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string, size int) string {
	t.Helper()
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte(i % 251)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	h := sha256.Sum256(buf)
	return hex.EncodeToString(h[:])
}

func TestResolveAdapterDir(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		if _, err := resolveAdapterDir(""); err == nil {
			t.Error("expected error on empty path")
		}
	})
	t.Run("nonexistent path", func(t *testing.T) {
		if _, err := resolveAdapterDir("/nonexistent/xxxxyyyyzzzz"); err == nil {
			t.Error("expected error on nonexistent path")
		}
	})
	t.Run("existing dir", func(t *testing.T) {
		d := t.TempDir()
		abs, err := resolveAdapterDir(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !filepath.IsAbs(abs) {
			t.Errorf("expected absolute path, got %s", abs)
		}
	})
	t.Run("path is file not dir", func(t *testing.T) {
		d := t.TempDir()
		fp := filepath.Join(d, "not-a-dir")
		_ = os.WriteFile(fp, []byte("hi"), 0o644)
		if _, err := resolveAdapterDir(fp); err == nil {
			t.Error("expected error when path is a file, not a directory")
		}
	})
}

func TestEnumerateCheckpoints(t *testing.T) {
	dir := t.TempDir()
	// 3 checkpoints + noise
	sha400 := writeFile(t, filepath.Join(dir, "0000400_adapters.safetensors"), 100)
	sha800 := writeFile(t, filepath.Join(dir, "0000800_adapters.safetensors"), 200)
	sha1200 := writeFile(t, filepath.Join(dir, "0001200_adapters.safetensors"), 300)
	// noise files ignored
	_ = writeFile(t, filepath.Join(dir, "adapters.safetensors"), 100)
	_ = writeFile(t, filepath.Join(dir, "adapter_config.json"), 20)
	_ = writeFile(t, filepath.Join(dir, "README.md"), 30)

	cps, err := enumerateCheckpoints(dir)
	if err != nil {
		t.Fatalf("enumerateCheckpoints: %v", err)
	}
	if len(cps) != 3 {
		t.Fatalf("want 3 checkpoints, got %d", len(cps))
	}
	if cps[0].Iter != 400 || cps[1].Iter != 800 || cps[2].Iter != 1200 {
		t.Errorf("want sorted [400,800,1200], got [%d,%d,%d]", cps[0].Iter, cps[1].Iter, cps[2].Iter)
	}
	if cps[0].SHA256 != sha400 || cps[1].SHA256 != sha800 || cps[2].SHA256 != sha1200 {
		t.Error("SHA mismatch")
	}
	if cps[0].Size != 100 || cps[2].Size != 300 {
		t.Errorf("size mismatch: got %d/%d", cps[0].Size, cps[2].Size)
	}
}

func TestEnumerateCheckpoints_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	cps, err := enumerateCheckpoints(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cps) != 0 {
		t.Errorf("want 0 checkpoints in empty dir, got %d", len(cps))
	}
}

func TestFindCheckpointByIter(t *testing.T) {
	dir := t.TempDir()
	_ = writeFile(t, filepath.Join(dir, "0000400_adapters.safetensors"), 10)
	_ = writeFile(t, filepath.Join(dir, "0001200_adapters.safetensors"), 20)

	c, err := findCheckpointByIter(dir, 1200)
	if err != nil {
		t.Fatalf("expected iter 1200 found: %v", err)
	}
	if c.Iter != 1200 {
		t.Errorf("want iter=1200, got %d", c.Iter)
	}

	if _, err := findCheckpointByIter(dir, 9999); err == nil {
		t.Error("expected error for missing iter 9999")
	}
}

func TestBenchServePidfileRoundTrip(t *testing.T) {
	// Redirect state dir into t.TempDir() via HOME env override.
	// (mdemgStateDir uses os.UserHomeDir.)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path, err := benchServePidFile(9999)
	if err != nil {
		t.Fatalf("benchServePidFile: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("want abs path, got %s", path)
	}

	rec := benchServePidRecord{
		PID:        12345,
		AdapterDir: "/tmp/foo",
		BaseModel:  "/tmp/base",
		Port:       9999,
		MdemgCmd:   "python -m mlx_lm.server ...",
	}
	if err := writePidRecord(path, rec); err != nil {
		t.Fatalf("writePidRecord: %v", err)
	}
	got, err := readPidRecord(path)
	if err != nil {
		t.Fatalf("readPidRecord: %v", err)
	}
	if got.PID != 12345 || got.AdapterDir != "/tmp/foo" || got.Port != 9999 {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	// readPidRecord on missing file
	_ = os.Remove(path)
	if _, err := readPidRecord(path); !os.IsNotExist(err) {
		t.Errorf("want os.IsNotExist, got %v", err)
	}
}

func TestSHA256File_Deterministic(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x")
	sha := writeFile(t, f, 1024)
	got, err := sha256File(f)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}
	if got != sha {
		t.Errorf("sha256File mismatch: %s != %s", got, sha)
	}
}

func TestCheckpointFileRE(t *testing.T) {
	cases := map[string]bool{
		"0000400_adapters.safetensors": true,
		"0001200_adapters.safetensors": true,
		"1_adapters.safetensors":       true, // any digit count matches
		"adapters.safetensors":         false,
		"adapter_config.json":          false,
		"README.md":                    false,
		"0000400_adapters.bin":         false,
	}
	for name, wantMatch := range cases {
		got := checkpointFileRE.MatchString(name)
		if got != wantMatch {
			t.Errorf("%q: want match=%v got=%v", name, wantMatch, got)
		}
	}
}
