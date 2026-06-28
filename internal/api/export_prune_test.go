package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPruneOldExports pins SF-6 (FT-RECURSIVE-001): stale export archives are
// removed, recent ones kept, non-archives left alone, and retention<=0 disables.
func TestPruneOldExports(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string, age time.Duration) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		mt := time.Now().Add(-age)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
		return p
	}
	oldArc := mk("exp-old.tar.gz", 200*time.Hour)
	newArc := mk("exp-new.tar.gz", 1*time.Hour)
	other := mk("keepme.txt", 200*time.Hour)

	pruneOldExports(dir, 168) // 7 days

	if _, err := os.Stat(oldArc); !os.IsNotExist(err) {
		t.Error("old archive should have been pruned")
	}
	if _, err := os.Stat(newArc); err != nil {
		t.Error("recent archive should be kept")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("non-.tar.gz files must not be pruned")
	}

	// retention<=0 disables: re-create an old archive, ensure it survives.
	oldArc2 := mk("exp-old2.tar.gz", 200*time.Hour)
	pruneOldExports(dir, 0)
	if _, err := os.Stat(oldArc2); err != nil {
		t.Error("retention<=0 should disable pruning")
	}
}
