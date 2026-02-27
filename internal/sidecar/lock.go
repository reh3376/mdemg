package sidecar

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const lockFileName = "sidecar.lock"

// LockFilePath returns the expected path for .mdemg/sidecar.lock relative to cwd.
func LockFilePath() string {
	abs, _ := filepath.Abs(filepath.Join(mdemgDir, lockFileName))
	return abs
}

// LockFilePathFrom returns the lock file path relative to the given directory.
func LockFilePathFrom(dir string) string {
	return filepath.Join(dir, mdemgDir, lockFileName)
}

// FindLockFile walks up from cwd looking for .mdemg/sidecar.lock.
func FindLockFile() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return FindLockFileFrom(dir)
}

// FindLockFileFrom walks up from the given directory looking for .mdemg/sidecar.lock.
func FindLockFileFrom(startDir string) string {
	dir := startDir
	for {
		candidate := filepath.Join(dir, mdemgDir, lockFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// ReadLock reads and parses a sidecar.lock file.
func ReadLock(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read lock: %w", err)
	}
	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse lock: %w", err)
	}
	return &lf, nil
}

// WriteLock serializes and atomically writes a sidecar.lock file.
func WriteLock(path string, lock *LockFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create lock dir: %w", err)
	}

	lock.UpdatedAt = NowUTC()

	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lock: %w", err)
	}
	data = append(data, '\n')

	// Atomic write: tmp file + rename
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write lock tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename lock: %w", err)
	}
	return nil
}

// CurrentState reads the lock file to determine the current sidecar state.
// Returns StateUninitialized if no lock file is found.
func CurrentState() State {
	path := FindLockFile()
	if path == "" {
		return StateUninitialized
	}
	lf, err := ReadLock(path)
	if err != nil {
		return StateUninitialized
	}
	return lf.State
}

// CurrentStateFrom reads the lock file relative to the given directory.
func CurrentStateFrom(dir string) State {
	path := FindLockFileFrom(dir)
	if path == "" {
		return StateUninitialized
	}
	lf, err := ReadLock(path)
	if err != nil {
		return StateUninitialized
	}
	return lf.State
}

// NewLockFile creates a fresh LockFile with the given parameters.
func NewLockFile(profile string, endpoint string, configHash string) *LockFile {
	now := NowUTC()
	return &LockFile{
		State:        StateInitialized,
		Profile:      profile,
		Endpoint:     endpoint,
		ConfigHash:   configHash,
		MdemgVersion: "dev", // overridden at build time
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
