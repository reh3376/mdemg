package ape

import (
	"context"
	"testing"
	"time"

	"mdemg/internal/config"
)

// ENFORCE-AUTO-EXECUTE — pin tests for the archive_constraint_by_code executor.

func newTestDispatcherWithCfg(cfg config.Config, mock *mockGuidanceCalibrator) *Dispatcher {
	d := &Dispatcher{}
	d.SetConfig(cfg)
	d.SetGuidanceCalibrator(mock)
	return d
}

func TestExecuteArchive_DisabledByDefault(t *testing.T) {
	// Default cfg: EnforcementAutoExecuteEnabled=false → skipped_disabled.
	d := newTestDispatcherWithCfg(config.Config{}, &mockGuidanceCalibrator{archiveByCodeFound: true, archiveByCodeArchived: true})
	got, err := d.executeArchiveConstraintByCode(context.Background(), "sp", "CODE-X")
	if err != nil {
		t.Fatal(err)
	}
	if got["result"] != "skipped_disabled" {
		t.Errorf("expected skipped_disabled, got %v", got["result"])
	}
}

func TestExecuteArchive_EmptyCodeSkipsBeforeGuards(t *testing.T) {
	// Empty TargetCode → skipped_no_code (even if enabled + not dry run).
	d := newTestDispatcherWithCfg(
		config.Config{EnforcementAutoExecuteEnabled: true, EnforcementAutoExecuteDryRun: false},
		&mockGuidanceCalibrator{},
	)
	got, err := d.executeArchiveConstraintByCode(context.Background(), "sp", "")
	if err != nil {
		t.Fatal(err)
	}
	if got["result"] != "skipped_no_code" {
		t.Errorf("expected skipped_no_code, got %v", got["result"])
	}
}

func TestExecuteArchive_ProtectedSpaceSkipsBeforeMutation(t *testing.T) {
	// spaceID in RSICProtectedSpaces → skipped_protected_space (even if enabled).
	d := newTestDispatcherWithCfg(
		config.Config{
			EnforcementAutoExecuteEnabled: true,
			EnforcementAutoExecuteDryRun:  false,
			RSICProtectedSpaces:           []string{"protected-space"},
		},
		&mockGuidanceCalibrator{},
	)
	got, err := d.executeArchiveConstraintByCode(context.Background(), "protected-space", "CODE-X")
	if err != nil {
		t.Fatal(err)
	}
	if got["result"] != "skipped_protected_space" {
		t.Errorf("expected skipped_protected_space, got %v", got["result"])
	}
}

func TestExecuteArchive_DryRunLogs(t *testing.T) {
	// Enabled + dry-run → dry_run_would_archive (no mutation).
	d := newTestDispatcherWithCfg(
		config.Config{
			EnforcementAutoExecuteEnabled:       true,
			EnforcementAutoExecuteDryRun:        true,
			EnforcementAutoExecuteMaxPerHour:    3,
			EnforcementAutoExecuteCooldownHours: 24,
		},
		&mockGuidanceCalibrator{archiveByCodeFound: true, archiveByCodeArchived: true},
	)
	got, err := d.executeArchiveConstraintByCode(context.Background(), "sp", "CODE-X")
	if err != nil {
		t.Fatal(err)
	}
	if got["result"] != "dry_run_would_archive" {
		t.Errorf("expected dry_run_would_archive, got %v", got["result"])
	}
}

func TestExecuteArchive_RealArchiveOnLiveConfig(t *testing.T) {
	// Enabled + NOT dry-run → real archive, returns archived=true.
	mock := &mockGuidanceCalibrator{archiveByCodeFound: true, archiveByCodeArchived: true}
	d := newTestDispatcherWithCfg(
		config.Config{
			EnforcementAutoExecuteEnabled:       true,
			EnforcementAutoExecuteDryRun:        false,
			EnforcementAutoExecuteMaxPerHour:    3,
			EnforcementAutoExecuteCooldownHours: 24,
		},
		mock,
	)
	got, err := d.executeArchiveConstraintByCode(context.Background(), "sp", "CODE-X")
	if err != nil {
		t.Fatal(err)
	}
	if got["result"] != "archived" {
		t.Errorf("expected archived, got %v", got["result"])
	}
	if got["archived"] != true {
		t.Errorf("expected archived=true, got %v", got["archived"])
	}
}

func TestExecuteArchive_PerCodeCooldown(t *testing.T) {
	// First real archive succeeds; immediate retry on SAME code → skipped_rate_limited (per-code cooldown).
	mock := &mockGuidanceCalibrator{archiveByCodeFound: true, archiveByCodeArchived: true}
	d := newTestDispatcherWithCfg(
		config.Config{
			EnforcementAutoExecuteEnabled:       true,
			EnforcementAutoExecuteDryRun:        false,
			EnforcementAutoExecuteMaxPerHour:    3,
			EnforcementAutoExecuteCooldownHours: 24,
		},
		mock,
	)
	first, _ := d.executeArchiveConstraintByCode(context.Background(), "sp", "CODE-COOLDOWN")
	if first["result"] != "archived" {
		t.Fatalf("first call should archive, got %v", first["result"])
	}
	second, _ := d.executeArchiveConstraintByCode(context.Background(), "sp", "CODE-COOLDOWN")
	if second["result"] != "skipped_rate_limited" {
		t.Errorf("second call on same code should be cooldown-blocked, got %v", second["result"])
	}
}

func TestExecuteArchive_GlobalRateLimit(t *testing.T) {
	// MaxPerHour=2 → three distinct codes: first two archive, third → skipped_rate_limited.
	mock := &mockGuidanceCalibrator{archiveByCodeFound: true, archiveByCodeArchived: true}
	d := newTestDispatcherWithCfg(
		config.Config{
			EnforcementAutoExecuteEnabled:       true,
			EnforcementAutoExecuteDryRun:        false,
			EnforcementAutoExecuteMaxPerHour:    2,
			EnforcementAutoExecuteCooldownHours: 24,
		},
		mock,
	)
	one, _ := d.executeArchiveConstraintByCode(context.Background(), "sp", "CODE-A")
	two, _ := d.executeArchiveConstraintByCode(context.Background(), "sp", "CODE-B")
	three, _ := d.executeArchiveConstraintByCode(context.Background(), "sp", "CODE-C")
	if one["result"] != "archived" || two["result"] != "archived" {
		t.Errorf("first two must archive, got %v / %v", one["result"], two["result"])
	}
	if three["result"] != "skipped_rate_limited" {
		t.Errorf("third code must be global-rate-limited, got %v", three["result"])
	}
}

func TestEnforceReserveSlot_SlidingWindowExpires(t *testing.T) {
	// Manually seed old timestamps (> 1h ago) → they're pruned + slot becomes available.
	d := &Dispatcher{}
	d.SetConfig(config.Config{EnforcementAutoExecuteMaxPerHour: 1})
	old := time.Now().Add(-2 * time.Hour)
	d.enforceRecentArchives = []time.Time{old}
	if !d.enforceReserveSlot("CODE-EXPIRY") {
		t.Error("stale timestamp (>1h) should not block a fresh reservation")
	}
	// Immediately try again: now blocked (fresh slot fills the quota of 1).
	if d.enforceReserveSlot("CODE-DIFFERENT") {
		t.Error("second reserve at MaxPerHour=1 must fail")
	}
}
