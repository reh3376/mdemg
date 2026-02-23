package migrations

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// MaxVersion returns the highest migration version found in the embedded FS.
// Migration filenames must follow the pattern V<NNNN>__<name>.cypher.
func MaxVersion() (int, error) {
	return maxVersionFromFS(FS)
}

// maxVersionFromFS extracts the maximum version from any fs.FS.
// Exported for testing with custom filesystems.
func maxVersionFromFS(fsys fs.FS) (int, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return 0, fmt.Errorf("read migrations dir: %w", err)
	}

	max := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".cypher") {
			continue
		}
		ver, err := parseVersion(e.Name())
		if err != nil {
			continue // skip non-conforming files
		}
		if ver > max {
			max = ver
		}
	}

	if max == 0 {
		return 0, fmt.Errorf("no migration files found")
	}
	return max, nil
}

// parseVersion extracts the integer version from a filename like "V0015__secondary_labels.cypher".
func parseVersion(filename string) (int, error) {
	if !strings.HasPrefix(filename, "V") {
		return 0, fmt.Errorf("filename does not start with V: %s", filename)
	}
	parts := strings.SplitN(filename[1:], "__", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("filename missing __ separator: %s", filename)
	}
	ver, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("parse version from %s: %w", filename, err)
	}
	return ver, nil
}
