// Package dockerbin resolves the `docker` CLI executable path robustly across
// launch environments.
//
// Why this exists: server-runtime code shells out to `docker` for optional
// telemetry (Neo4j container stats) and for scheduled TSDB backups (`docker
// compose … pg_dump`). When the process is started by launchd/systemd it
// inherits a minimal PATH (`/usr/bin:/bin:/usr/sbin:/sbin`) that excludes the
// Docker Desktop symlink (`/usr/local/bin/docker`) and the Apple-Silicon
// Homebrew location (`/opt/homebrew/bin/docker`). A bare `exec.Command("docker",
// …)` then fails with "executable file not found in $PATH" even though the data
// plane (Neo4j Bolt + TSDB pgx over mapped ports) is completely healthy.
//
// Resolution order (single source of truth; no hardcoded literal in callers):
//  1. MDEMG_DOCKER_BIN env var (explicit operator override — wins always)
//  2. exec.LookPath("docker") (the inherited PATH, when it works)
//  3. well-known install locations (Docker Desktop macOS, Apple-Silicon brew, Linux)
//
// When nothing resolves, Path() returns "docker" as a last resort so behavior
// matches the historical bare-exec (callers still handle the error), and
// Available() reports false so callers can degrade gracefully instead of
// erroring on a loop.
package dockerbin

import (
	"os"
	"os/exec"
	"sync"
)

// EnvOverride is the env var an operator can set to pin the docker binary path
// (e.g. an unusual install location or a wrapper script).
const EnvOverride = "MDEMG_DOCKER_BIN"

// wellKnownPaths are probed in order when the binary isn't on PATH. Covers
// Docker Desktop on macOS, Homebrew on Apple Silicon, and standard Linux.
var wellKnownPaths = []string{
	"/usr/local/bin/docker",   // Docker Desktop (macOS) symlink
	"/opt/homebrew/bin/docker", // Homebrew on Apple Silicon
	"/usr/bin/docker",          // Linux distro packages
	"/usr/local/sbin/docker",
}

var (
	cacheOnce sync.Once
	cachedAbs string // resolved absolute path, or "" if none found
)

// resolve does the actual lookup. Cached after first call (the environment
// doesn't change within a process lifetime; an explicit override is read once).
func resolve() string {
	if override := os.Getenv(EnvOverride); override != "" {
		return override
	}
	if p, err := exec.LookPath("docker"); err == nil {
		return p
	}
	for _, c := range wellKnownPaths {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

func cached() string {
	cacheOnce.Do(func() { cachedAbs = resolve() })
	return cachedAbs
}

// Path returns a usable docker executable path. When the binary cannot be
// located it returns "docker" (the historical bare value) so the subsequent
// exec produces the same not-found error a caller already handles — callers
// that want to avoid the attempt entirely should gate on Available().
func Path() string {
	if abs := cached(); abs != "" {
		return abs
	}
	return "docker"
}

// Available reports whether a docker executable was located. Callers use this
// to skip optional docker work (and log once) rather than failing on a loop.
func Available() bool {
	return cached() != ""
}
