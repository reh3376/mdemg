// Package cli — `mdemg adapter freeze`
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

const (
	adapterMainFile   = "adapters.safetensors"
	adapterBackupFile = "adapters.safetensors.pre-freeze"
	freezeAuditFile   = "freeze_log.jsonl"
)

func newAdapterFreezeCmd() *cobra.Command {
	var (
		dir  string
		iter int
		yes  bool
	)
	cmd := &cobra.Command{
		Use:   "freeze",
		Short: "Pin a specific checkpoint iter as adapters.safetensors",
		Long: `Copy adapters/<name>/000NNNN_adapters.safetensors → adapters/<name>/adapters.safetensors.

If adapters.safetensors already exists in the target dir, it is backed up to
adapters.safetensors.pre-freeze (reversible; only ONE backup slot per dir —
subsequent freezes overwrite it, so preserve the file yourself if needed).

Every freeze appends a row to freeze_log.jsonl in the adapter dir with the
iter, SHA-256 of the pinned checkpoint, and timestamp.

Example:
  mdemg adapter freeze --dir adapters/phase_e3_v1_base_v3 --iter 1200 --yes
`,
		RunE: func(_ *cobra.Command, _ []string) error {
			abs, err := resolveAdapterDir(dir)
			if err != nil {
				return err
			}
			cp, err := findCheckpointByIter(abs, iter)
			if err != nil {
				return err
			}

			mainPath := filepath.Join(abs, adapterMainFile)
			backupPath := filepath.Join(abs, adapterBackupFile)

			var backupSHA string
			var backedUp bool
			if _, err := os.Stat(mainPath); err == nil {
				if !yes {
					return fmt.Errorf("adapters.safetensors already exists at %s; pass --yes to overwrite (a .pre-freeze backup will be captured)", mainPath)
				}
				preSHA, err := sha256File(mainPath)
				if err != nil {
					return fmt.Errorf("hash existing adapters.safetensors: %w", err)
				}
				if err := copyFileForFreeze(mainPath, backupPath); err != nil {
					return fmt.Errorf("backup existing adapters.safetensors: %w", err)
				}
				backupSHA = preSHA
				backedUp = true
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("stat adapters.safetensors: %w", err)
			}

			if err := copyFileForFreeze(cp.Path, mainPath); err != nil {
				return fmt.Errorf("copy checkpoint: %w", err)
			}
			postSHA, err := sha256File(mainPath)
			if err != nil {
				return fmt.Errorf("verify frozen adapters.safetensors: %w", err)
			}
			if postSHA != cp.SHA256 {
				return fmt.Errorf("freeze verify failed: checkpoint sha=%s frozen sha=%s", cp.SHA256, postSHA)
			}

			row := freezeAuditRow{
				Iter:         cp.Iter,
				CheckpointSH: cp.SHA256,
				FrozenAt:     time.Now().UTC(),
			}
			if backedUp {
				row.BackupPath = backupPath
				row.BackupSHA = backupSHA
			}
			if err := appendFreezeAudit(filepath.Join(abs, freezeAuditFile), row); err != nil {
				return fmt.Errorf("append freeze audit: %w", err)
			}

			out := map[string]any{
				"dir":               abs,
				"frozen_iter":       cp.Iter,
				"frozen_sha256":     cp.SHA256,
				"main_path":         mainPath,
				"backup_captured":   backedUp,
				"backup_path":       backupPath,
				"backup_sha256":     backupSHA,
				"audit_appended_to": filepath.Join(abs, freezeAuditFile),
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "adapter directory (required)")
	cmd.Flags().IntVar(&iter, "iter", 0, "checkpoint iter to freeze (required)")
	cmd.Flags().BoolVar(&yes, "yes", false, "overwrite existing adapters.safetensors (captures a .pre-freeze backup)")
	_ = cmd.MarkFlagRequired("dir")
	_ = cmd.MarkFlagRequired("iter")
	return cmd
}

func copyFileForFreeze(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // adapter safetensors: match source 0o644 so downstream mlx_lm.server can read
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func appendFreezeAudit(path string, row freezeAuditRow) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // audit log co-located with adapter files at same 0o644 mode
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(row)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}
