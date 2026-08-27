// Package cli — ADAPTER-SWAP-STANDARDIZE-001 shared helpers.
//
// Sprint: docs/development/adapter-swap-standardize-001/sprint_plan.md
package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// checkpoint describes one saved LoRA adapter file in an adapter directory.
type checkpoint struct {
	Iter   int       `json:"iter"`
	Path   string    `json:"path"`
	Size   int64     `json:"size"`
	SHA256 string    `json:"sha256"`
	MTime  time.Time `json:"mtime"`
}

// freezeAuditRow records one freeze operation for the append-only
// `freeze_log.jsonl` at the adapter dir.
type freezeAuditRow struct {
	Iter         int       `json:"iter"`
	CheckpointSH string    `json:"checkpoint_sha256"`
	FrozenAt     time.Time `json:"frozen_at_utc"`
	BackupPath   string    `json:"backup_path,omitempty"`
	BackupSHA    string    `json:"backup_sha256,omitempty"`
}

var checkpointFileRE = regexp.MustCompile(`^(\d+)_adapters\.safetensors$`)

// resolveAdapterDir cleans an operator-supplied adapter dir path,
// making it absolute + verifying it exists as a directory.
func resolveAdapterDir(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("adapter dir path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve adapter dir: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("adapter dir not found: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("adapter path is not a directory: %s", abs)
	}
	return abs, nil
}

// enumerateCheckpoints lists all `NNNN_adapters.safetensors` files
// in the adapter dir, sorted by iter number ascending.
// Files with `adapters.safetensors` (no iter prefix) are the FROZEN
// checkpoint pointer and are NOT listed as candidates.
func enumerateCheckpoints(dir string) ([]checkpoint, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read adapter dir: %w", err)
	}
	var out []checkpoint
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := checkpointFileRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		iter, _ := strconv.Atoi(m[1])
		info, err := e.Info()
		if err != nil {
			continue
		}
		full := filepath.Join(dir, e.Name())
		sha, err := sha256File(full)
		if err != nil {
			return nil, fmt.Errorf("hash %s: %w", full, err)
		}
		out = append(out, checkpoint{
			Iter:   iter,
			Path:   full,
			Size:   info.Size(),
			SHA256: sha,
			MTime:  info.ModTime().UTC(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Iter < out[j].Iter })
	return out, nil
}

// findCheckpointByIter returns the checkpoint matching the given iter
// or an error if not present in the dir.
func findCheckpointByIter(dir string, iter int) (checkpoint, error) {
	cps, err := enumerateCheckpoints(dir)
	if err != nil {
		return checkpoint{}, err
	}
	for _, c := range cps {
		if c.Iter == iter {
			return c, nil
		}
	}
	return checkpoint{}, fmt.Errorf("no checkpoint with iter=%d in %s", iter, dir)
}

// sha256File returns the hex sha256 of a file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// mdemgStateDir returns `~/.mdemg` (creates on demand). Kept ~identical
// to sibling CLI patterns (see internal/cli/beta_share.go usage).
func mdemgStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".mdemg")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// benchServePidFile returns the path to the pidfile for a given bench-serve port.
// One pidfile per port; multiple bench-serve instances on different ports
// coexist.
func benchServePidFile(port int) (string, error) {
	dir, err := mdemgStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("bench-serve-%d.json", port)), nil
}

// benchServePidRecord is what we persist to the pidfile — pid + adapter
// dir + timestamp, so `--stop` can verify the process still matches what
// we spawned and doesn't kill an unrelated pid.
type benchServePidRecord struct {
	PID         int       `json:"pid"`
	AdapterDir  string    `json:"adapter_dir"`
	BaseModel   string    `json:"base_model"`
	Port        int       `json:"port"`
	StartedAt   time.Time `json:"started_at_utc"`
	MdemgCmd    string    `json:"mdemg_cmd"`
}

func writePidRecord(path string, rec benchServePidRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readPidRecord(path string) (benchServePidRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return benchServePidRecord{}, err
	}
	var rec benchServePidRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return benchServePidRecord{}, fmt.Errorf("parse pidfile %s: %w", path, err)
	}
	return rec, nil
}
