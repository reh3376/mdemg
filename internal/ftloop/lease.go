// Compute lease + disk floor for the recursive-retrain controller
// (FT-RECURSIVE-002 Phase 6b, Epic 4). The lease is a single-host exclusive
// mutex (a lockfile with an expiry) that the controller holds across a training
// run so a crashed trainer cannot wedge RSIC forever — an expired lease is
// reclaimable, and the controller halts (class-4 alert) on its own lease
// expiring mid-run.
package ftloop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ErrLeaseHeld is returned when a non-expired lease already exists.
var ErrLeaseHeld = errors.New("ftloop: compute lease already held")

// leaseFile is the on-disk lease record.
type leaseFile struct {
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	PID        int       `json:"pid"`
	CycleID    string    `json:"cycle_id"`
}

// Lease is a held compute lease.
type Lease struct {
	path      string
	expiresAt time.Time
}

// AcquireLease takes the exclusive compute lease at `path`, valid for `maxDur`.
// Fails with ErrLeaseHeld if a non-expired lease already exists (single-flight
// at the OS level, complementing the DB ledger). An expired or absent lease is
// reclaimed. cycleID is recorded for forensics.
func AcquireLease(path, cycleID string, maxDur time.Duration, now time.Time) (*Lease, error) {
	if path == "" {
		return nil, errors.New("ftloop: empty lease path")
	}
	if existing, err := readLease(path); err == nil {
		if now.Before(existing.ExpiresAt) {
			return nil, fmt.Errorf("%w (cycle=%s, expires=%s)", ErrLeaseHeld, existing.CycleID, existing.ExpiresAt.Format(time.RFC3339))
		}
		// expired → reclaimable
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("ftloop: lease dir: %w", err)
	}
	expires := now.Add(maxDur)
	lf := leaseFile{AcquiredAt: now, ExpiresAt: expires, PID: os.Getpid(), CycleID: cycleID}
	b, _ := json.Marshal(lf)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, fmt.Errorf("ftloop: write lease: %w", err)
	}
	return &Lease{path: path, expiresAt: expires}, nil
}

// Release removes the lease file. Safe to call multiple times.
func (l *Lease) Release() error {
	if l == nil {
		return nil
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Expired reports whether the lease's max duration has elapsed (the controller
// must halt + class-4 alert if its own lease expires mid-run).
func (l *Lease) Expired(now time.Time) bool {
	return l != nil && now.After(l.expiresAt)
}

// ExpiresAt returns the lease expiry.
func (l *Lease) ExpiresAt() time.Time {
	if l == nil {
		return time.Time{}
	}
	return l.expiresAt
}

func readLease(path string) (leaseFile, error) {
	var lf leaseFile
	b, err := os.ReadFile(path)
	if err != nil {
		return lf, err
	}
	if err := json.Unmarshal(b, &lf); err != nil {
		return lf, err
	}
	return lf, nil
}

// FreeDiskGB returns the free space (GiB) on the filesystem containing `path`.
// Used for the FT_LOOP_MIN_FREE_DISK_GB preflight floor (a training run needs
// ~85 GB transient disk per the Phase-5 actuals).
func FreeDiskGB(path string) (float64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	free := float64(st.Bavail) * float64(st.Bsize)
	return free / (1 << 30), nil
}
