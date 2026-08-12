// Package pathsafe provides CodeQL-recognizable path-injection sanitizers.
//
// SEC-TRANCHE-2 (2026-08-11): CodeQL's Go path-injection detector does NOT
// recognize regex-based validation as sanitization (e.g. the shipped
// `validatePluginID` in internal/api/plugin_handlers.go). The pattern it
// DOES recognize is `filepath.Clean` followed by a containment check
// (strings.HasPrefix against a trusted base). This package is that pattern,
// centralized so every HTTP-facing site can call one function and the
// detector's data-flow analysis closes.
//
// Two rules pinned by SEC-TRANCHE-2:
//  1. CodeQL doesn't recognize regex validation as a path-injection
//     sanitizer. Use SafeJoinUnderDir for HTTP-facing path construction.
//  2. `#nosec` is gosec-only; CodeQL ignores it. Use this helper or
//     dismiss via the GitHub code-scanning API with a rationale.
package pathsafe

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrTraversal is returned when the untrusted input contains a `..`
// segment. Callers can errors.Is against this for HTTP-400 mapping.
var ErrTraversal = errors.New("path segment contains ..")

// ErrEscape is returned when the joined path resolves outside baseDir
// after canonicalization. Callers can errors.Is for HTTP-400 mapping.
var ErrEscape = errors.New("resolved path escapes base directory")

// ErrEmptyBase is returned when baseDir is the empty string. A missing
// base is a programming error, not a caller error — surface it loudly
// rather than silently joining to "" which resolves to the process cwd.
var ErrEmptyBase = errors.New("baseDir is empty")

// SafeJoinUnderDir joins untrusted onto baseDir and returns the joined
// path only if it resolves strictly inside baseDir. Rejects `..`
// segments before Join (Clean would flatten them but their presence
// signals malicious intent), canonicalizes both sides via
// filepath.EvalSymlinks when they exist (macOS /var/folders →
// /private/var/folders class — see PLUGIN-PATH-INJECTION-FIX
// 2026-08-10), and applies a strict-prefix containment check.
//
// Use this at every HTTP handler that constructs a filesystem path from
// URL segments or JSON fields. CodeQL recognizes the
// filepath.Clean + strings.HasPrefix pattern as sanitizing the taint
// flow; regex validation at a distant caller does NOT close it.
func SafeJoinUnderDir(baseDir, untrusted string) (string, error) {
	if baseDir == "" {
		return "", ErrEmptyBase
	}
	// Reject explicit traversal before Join. filepath.Clean would
	// flatten `foo/../bar` to `bar` (potentially hiding intent);
	// rejecting the raw fragment catches the class earlier and gives
	// a clearer error message to the caller.
	if strings.Contains(untrusted, "..") {
		return "", ErrTraversal
	}
	// Absolute paths in the untrusted segment escape the base by
	// construction — filepath.Join(baseDir, "/etc/passwd") on unix
	// resolves to /etc/passwd (Go documents this).
	if filepath.IsAbs(untrusted) {
		return "", ErrEscape
	}
	joined := filepath.Clean(filepath.Join(baseDir, untrusted))
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	// Canonicalize via EvalSymlinks where the path exists. macOS
	// resolves /var/folders/* → /private/var/folders/*; without this
	// step a valid path can be misclassified as an escape when either
	// side goes through a symlink. The base MUST exist (constructor
	// responsibility); the target may not (caller can create it). To
	// keep the comparison apples-to-apples when the target doesn't yet
	// exist, canonicalize both sides by resolving the longest existing
	// ancestor and re-appending the non-existent tail. Ignore
	// not-exist errors from EvalSymlinks — they indicate the target
	// hasn't been created yet, not a validation failure.
	absBase = canonicalize(absBase)
	absJoined = canonicalize(absJoined)
	// Strict-prefix containment: joined MUST be inside baseDir, with
	// a separator boundary (so `/foo/bar` does not match base `/foo/b`).
	// Equality is permitted (untrusted == "" or "."), though callers
	// typically require a non-empty subpath.
	sep := string(os.PathSeparator)
	if absJoined != absBase && !strings.HasPrefix(absJoined+sep, absBase+sep) {
		return "", ErrEscape
	}
	return absJoined, nil
}

// canonicalize resolves symlinks in the longest existing prefix of p
// and re-appends any non-existent tail. This lets the containment
// check compare apples-to-apples even when the target doesn't yet
// exist (e.g. a fresh write). Fails soft: if no ancestor exists, or
// EvalSymlinks fails for a non-not-exist reason, returns p unchanged.
func canonicalize(p string) string {
	// Fast path: if the full path resolves, done.
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	// Walk up the parent chain until an ancestor resolves. This is
	// bounded by the number of separators in p.
	cur := p
	var tail []string
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			// Reached the root without an existing ancestor; give up.
			return p
		}
		base := filepath.Base(cur)
		tail = append([]string{base}, tail...)
		cur = parent
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(append([]string{resolved}, tail...)...)
		}
	}
}
