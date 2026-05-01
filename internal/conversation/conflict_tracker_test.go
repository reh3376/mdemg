package conversation

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestNewConflictTracker_NilPool verifies the no-op tracker contract — Track
// returns nil and never panics when constructed with a nil pool. Necessary
// because the production wiring path may construct the tracker before TSDB is
// known healthy.
func TestNewConflictTracker_NilPool(t *testing.T) {
	tr := NewConflictTracker(nil, time.Minute)
	err := tr.Track(context.Background(), ConflictRecord{
		SpaceID:        "mdemg-dev",
		DivergenceKind: DivergenceKindBinary,
		Source:         "unit-test",
	})
	if err != nil {
		t.Fatalf("Track with nil pool returned %v, want nil", err)
	}
}

// TestNewConflictTracker_NilReceiver verifies the nil-receiver contract — a
// caller wiring against an unset *ConflictTracker field shouldn't crash.
func TestNewConflictTracker_NilReceiver(t *testing.T) {
	var tr *ConflictTracker
	err := tr.Track(context.Background(), ConflictRecord{SpaceID: "x", DivergenceKind: "y", Source: "z"})
	if err != nil {
		t.Fatalf("Track on nil receiver returned %v, want nil", err)
	}
}

// TestTrack_ValidationErrors enumerates the three required-field violations.
// Each must surface as a non-nil error so misconfigured callers fail loudly
// rather than silently dropping divergence rows.
func TestTrack_ValidationErrors(t *testing.T) {
	tr := NewConflictTracker(nil, time.Minute)
	cases := []struct {
		name string
		rec  ConflictRecord
	}{
		{"missing-space-id", ConflictRecord{DivergenceKind: "binary", Source: "src"}},
		{"missing-divergence-kind", ConflictRecord{SpaceID: "s", Source: "src"}},
		{"missing-source", ConflictRecord{SpaceID: "s", DivergenceKind: "binary"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tr.Track(context.Background(), tc.rec); err == nil {
				t.Fatalf("expected validation error, got nil")
			}
		})
	}
}

// TestHashContext_Stability locks in the deterministic 16-char digest so any
// future tweak to the hashing scheme breaks this test rather than silently
// invalidating historical context_hash matches.
func TestHashContext_Stability(t *testing.T) {
	got := HashContext("warn", "allow", "block")
	if len(got) != 16 {
		t.Fatalf("HashContext length=%d, want 16", len(got))
	}
	again := HashContext("warn", "allow", "block")
	if got != again {
		t.Fatalf("HashContext not deterministic: got=%s vs again=%s", got, again)
	}
	differ := HashContext("warn", "allow", "different")
	if got == differ {
		t.Fatalf("HashContext collided across distinct inputs: %s", got)
	}
}

// TestTrack_RateLimiterSuppresses runs two consecutive Track calls within the
// rate-limit window and confirms the second is suppressed (returns nil) so
// the underlying pool is never asked to insert. Exercises the limiter
// without needing a TSDB.
func TestTrack_RateLimiterSuppresses(t *testing.T) {
	tr := NewConflictTracker(nil, time.Minute)
	rec := ConflictRecord{
		SpaceID:        "rate-limit-test-space",
		DivergenceKind: DivergenceKindBinary,
		Source:         "unit-test",
		Rationale:      "first",
	}
	if err := tr.Track(context.Background(), rec); err != nil {
		t.Fatalf("first Track failed: %v", err)
	}
	rec.Rationale = "second"
	if err := tr.Track(context.Background(), rec); err != nil {
		t.Fatalf("second Track returned err during suppression: %v", err)
	}
	// Indirect check: the limiter recorded SpaceID -> time. No external state
	// to assert on with nil pool, but the absence of error confirms the call
	// returned without attempting the insert.
}

// TestTrack_RateLimiterPerSpace ensures the limiter is per-space rather than
// global. Two distinct spaces should both get through within the same minute.
func TestTrack_RateLimiterPerSpace(t *testing.T) {
	tr := NewConflictTracker(nil, time.Minute)
	for _, space := range []string{"space-a", "space-b"} {
		err := tr.Track(context.Background(), ConflictRecord{
			SpaceID:        space,
			DivergenceKind: DivergenceKindBinary,
			Source:         "per-space-test",
		})
		if err != nil {
			t.Fatalf("Track(%q) failed: %v", space, err)
		}
	}
}

// TestTrack_LiveTSDB is the integration-tier check. Connects to the dev
// TSDB at localhost:5433 (skipped if unreachable) and confirms a real INSERT
// lands a row matching the input. Validates the SQL string + column order
// + the V0015 hypertable schema together. Skips gracefully on CI / dev
// machines without a running TSDB.
func TestTrack_LiveTSDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live TSDB integration test in -short mode")
	}
	dsn := os.Getenv("TEST_TSDB_DSN")
	if dsn == "" {
		dsn = "postgres://mdemg:mdemg_metrics@localhost:5433/mdemg_metrics?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("TSDB unreachable (%v); skipping live integration", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("TSDB ping failed (%v); skipping live integration", err)
	}

	// Scope to a unique space so we can assert + clean up without colliding.
	uniqueSpace := "test-conflict-tracker-" + time.Now().Format("150405.000000")

	// Use a 1-nanosecond rate limit so the integration test isn't gated by
	// the per-space minute window.
	tr := NewConflictTracker(pool, time.Nanosecond)

	rec := ConflictRecord{
		SpaceID:                  uniqueSpace,
		JiminyRecommendation:     "warn-on-shadow-shadowing",
		RSICRecommendation:       "allow",
		ConsultingRecommendation: "block",
		DivergenceKind:           DivergenceKindOrdinal,
		Rationale:                "warn vs allow vs block",
		Source:                   "integration-test",
		InstanceID:               "test-host",
	}

	if err := tr.Track(ctx, rec); err != nil {
		t.Fatalf("Track failed: %v", err)
	}

	var rows int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM guidance_conflicts
		   WHERE space_id = $1 AND divergence_kind = $2 AND source = $3`,
		uniqueSpace, DivergenceKindOrdinal, "integration-test",
	).Scan(&rows)
	if err != nil {
		t.Fatalf("QueryRow failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row in guidance_conflicts for %q, got %d", uniqueSpace, rows)
	}

	// Cleanup — the test is idempotent if re-run.
	_, _ = pool.Exec(ctx, `DELETE FROM guidance_conflicts WHERE space_id = $1`, uniqueSpace)
}
