// INGEST-EXEC-001 — resolve the mdemg binary for server-triggered ingest.
//
// Both ingest-job paths exec'd a hardcoded relative "./bin/mdemg": broken in
// Docker (the documented-primary deployment ships the binary at /usr/local/
// bin/mdemg with no repo checkout) and in any working directory other than
// the repo root. Resolution order, env-first per the no-hardcoding rule:
//
//  1. MDEMG_BIN env override (operator escape hatch / containers)
//  2. os.Executable() — the running server IS the mdemg binary
//  3. PATH lookup ("mdemg")
//  4. ./bin/mdemg (legacy dev fallback)
package api

import (
	"os"
	"os/exec"
	"sync"
)

var (
	mdemgBinOnce sync.Once
	mdemgBinPath string
)

// resolveMdemgBin returns the mdemg binary path, cached after first call.
func resolveMdemgBin() string {
	mdemgBinOnce.Do(func() {
		if env := os.Getenv("MDEMG_BIN"); env != "" {
			if _, err := os.Stat(env); err == nil {
				mdemgBinPath = env
				return
			}
		}
		if self, err := os.Executable(); err == nil && self != "" {
			mdemgBinPath = self
			return
		}
		if p, err := exec.LookPath("mdemg"); err == nil {
			mdemgBinPath = p
			return
		}
		mdemgBinPath = "./bin/mdemg"
	})
	return mdemgBinPath
}
