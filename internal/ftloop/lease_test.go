package ftloop

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLease_AcquireReleaseExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ft-loop.lease")
	now := time.Now()

	l1, err := AcquireLease(path, "cycle-1", time.Hour, now)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	// Second acquire while held → ErrLeaseHeld.
	if _, err := AcquireLease(path, "cycle-2", time.Hour, now.Add(time.Minute)); err == nil {
		t.Error("second acquire should fail while held")
	}
	// Not expired now; expired after maxDur.
	if l1.Expired(now.Add(30 * time.Minute)) {
		t.Error("lease should not be expired at 30m")
	}
	if !l1.Expired(now.Add(2 * time.Hour)) {
		t.Error("lease should be expired at 2h")
	}
	// An expired lease is reclaimable by a new acquire.
	if _, err := AcquireLease(path, "cycle-3", time.Hour, now.Add(2*time.Hour)); err != nil {
		t.Errorf("expired lease should be reclaimable: %v", err)
	}
	// Release makes it re-acquirable.
	l4, _ := AcquireLease(path, "cycle-4", time.Hour, now.Add(3*time.Hour))
	if err := l4.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := AcquireLease(path, "cycle-5", time.Hour, now.Add(3*time.Hour)); err != nil {
		t.Errorf("released lease should be re-acquirable: %v", err)
	}
}

func TestFreeDiskGB(t *testing.T) {
	gb, err := FreeDiskGB(t.TempDir())
	if err != nil {
		t.Fatalf("FreeDiskGB: %v", err)
	}
	if gb <= 0 {
		t.Errorf("free disk should be > 0, got %.2f", gb)
	}
}
