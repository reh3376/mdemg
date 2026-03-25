package main

import (
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// VaultWalker traverses an Obsidian vault directory, collecting markdown files
// while respecting directory exclusions.
type VaultWalker struct {
	RootPath    string
	ExcludeDirs map[string]bool
}

// VaultFile represents a single markdown file discovered in the vault.
type VaultFile struct {
	RelPath string
	AbsPath string
	ModTime time.Time
	Size    int64
}

// NewVaultWalker creates a VaultWalker rooted at rootPath. The .obsidian and
// .trash directories are always excluded in addition to any caller-supplied
// directory names.
func NewVaultWalker(rootPath string, excludeDirs []string) *VaultWalker {
	exc := map[string]bool{
		".obsidian": true,
		".trash":    true,
	}
	for _, d := range excludeDirs {
		exc[d] = true
	}
	return &VaultWalker{
		RootPath:    rootPath,
		ExcludeDirs: exc,
	}
}

// Walk traverses the vault and returns every .md file found, sorted by
// modification time descending (most recent first).
func (w *VaultWalker) Walk() ([]VaultFile, error) {
	var files []VaultFile

	err := filepath.WalkDir(w.RootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk error", "path", path, "err", err)
			return err
		}

		if d.IsDir() {
			name := d.Name()
			if w.ExcludeDirs[name] && path != w.RootPath {
				slog.Debug("skipping excluded directory", "dir", name)
				return fs.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			slog.Warn("stat error", "path", path, "err", err)
			return nil
		}

		rel, err := filepath.Rel(w.RootPath, path)
		if err != nil {
			return err
		}

		files = append(files, VaultFile{
			RelPath: rel,
			AbsPath: path,
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})

	slog.Debug("vault walk complete", "root", w.RootPath, "files", len(files))
	return files, nil
}

// filterModifiedSince returns only the files whose ModTime is strictly after
// the provided timestamp.
func filterModifiedSince(files []VaultFile, since time.Time) []VaultFile {
	var out []VaultFile
	for _, f := range files {
		if f.ModTime.After(since) {
			out = append(out, f)
		}
	}
	return out
}
