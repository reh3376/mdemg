// Package grafana_templates embeds the deploy/docker/grafana/ provisioning
// tree (dashboards + datasources + alerting + notifiers + dashboards.yml)
// so `mdemg init` can materialize them into a fresh install's cwd. Without
// this, a fresh Homebrew install's compose template mounts
// ./deploy/docker/grafana/... from paths that don't exist → Grafana boots
// blank. RELEASE-HYGIENE-001 E2.
//
// Source-of-truth is `deploy/docker/grafana/**` at the repo root
// (checked in; used by the repo's own dev docker-compose.yml). Because
// //go:embed can't cross package boundaries or use `..`, we mirror the
// tree into `staged/` and keep the two in sync via a Makefile target
// + CI drift check. Editing the canonical files without running
// `make sync-grafana-embed` will FAIL CI at the drift check.
package grafana_templates

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FS is the embedded Grafana provisioning tree. Mirror of
// `deploy/docker/grafana/**` at the repo root.
//
//go:embed all:staged
var FS embed.FS

// StagedRoot is the in-embed root directory containing the mirrored tree.
// Matches the go:embed target. Exposed for tests.
const StagedRoot = "staged"

// DeployRelPath is where Materialize writes files on disk, relative to the
// destination directory. Matches the compose template's cwd-relative mount
// paths (e.g. "./deploy/docker/grafana/provisioning/...").
const DeployRelPath = "deploy/docker/grafana"

// Materialize writes every embedded Grafana file to
// <destDir>/deploy/docker/grafana/**. Idempotent: files already present with
// matching sha256 are left alone (operator edits preserved). Returns the
// number of files written + the number preserved (skipped identical).
//
// Non-idempotent case: if a file exists but has DIFFERENT content than the
// embed, it's an operator edit and Materialize DOES NOT overwrite it —
// caller can force via ForceMaterialize (not exposed by mdemg init).
func Materialize(destDir string) (written, preserved int, err error) {
	if destDir == "" {
		return 0, 0, fmt.Errorf("grafana_templates: destDir is required")
	}
	err = fs.WalkDir(FS, StagedRoot, func(path string, d fs.DirEntry, wErr error) error {
		if wErr != nil {
			return wErr
		}
		if d.IsDir() {
			return nil
		}
		relFromRoot := strings.TrimPrefix(path, StagedRoot+"/")
		dst := filepath.Join(destDir, DeployRelPath, relFromRoot)
		embedBytes, rErr := FS.ReadFile(path)
		if rErr != nil {
			return fmt.Errorf("read embed %s: %w", path, rErr)
		}
		// Idempotency: if the file exists and matches the embed sha256,
		// treat as already-materialized. If it exists with DIFFERENT
		// content, treat as an operator edit and preserve.
		if existing, sErr := os.ReadFile(dst); sErr == nil {
			if sha(existing) == sha(embedBytes) {
				preserved++
				return nil
			}
			// operator-edited — preserve without overwrite
			preserved++
			return nil
		}
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), mkErr)
		}
		if wErr := os.WriteFile(dst, embedBytes, 0o644); wErr != nil {
			return fmt.Errorf("write %s: %w", dst, wErr)
		}
		written++
		return nil
	})
	return written, preserved, err
}

// Manifest returns the list of embedded file paths (relative to StagedRoot).
// Exposed for tests + CI drift verification.
func Manifest() ([]string, error) {
	var out []string
	err := fs.WalkDir(FS, StagedRoot, func(path string, d fs.DirEntry, wErr error) error {
		if wErr != nil {
			return wErr
		}
		if d.IsDir() {
			return nil
		}
		out = append(out, strings.TrimPrefix(path, StagedRoot+"/"))
		return nil
	})
	return out, err
}

// sha returns the sha256 of b as a hex string. Package-private helper.
func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
