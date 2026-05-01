// Package conversation — conflict_tracker.go (Phase 11.6.x — Action 1).
//
// Records divergent recommendations from Jiminy / RSIC / Consulting on the
// same context to TSDB so a 3-month observation window can answer the
// empirical question "do these subsystems disagree often enough to justify
// the FEP capstone in Note 09?". Without longitudinal divergence data, the
// FEP unification argument has no forcing function.
//
// Design notes:
//   - Track() is the only public entry point. Callers are the decision sites
//     in jiminy / ape (RSIC) / consulting. This sprint ships the recorder;
//     wiring those callers is queued for the next sprint per the post-doc.
//   - We rate-limit at one row per (space_id, minute) on the writer side so a
//     hot session can't balloon `guidance_conflicts`. The limiter is in-process
//     (sync.Map of last-time-per-space). On binary restart the limiter resets,
//     which is acceptable — divergence data is statistical, not transactional.
//   - We write directly via pgxpool.Exec (no batch) — at the expected rate
//     (≤1 row/space/minute) the per-row cost is negligible.
//   - On TSDB unavailable (nil pool), Track() returns nil so callers never
//     have their critical paths blocked by an analytics writer. Drops a
//     warning log so the absence of conflict rows isn't silently masked.
package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nrednav/cuid2"
)

// ConflictRecord is the input shape for Track. Callers fill the fields they
// know about; nil/empty Jiminy/RSIC/Consulting recommendations are valid (a
// silent subsystem is itself informative — encoded as DivergenceKindAbsent).
type ConflictRecord struct {
	SpaceID                  string
	ContextHash              string
	JiminyRecommendation     string
	RSICRecommendation       string
	ConsultingRecommendation string
	DivergenceKind           string
	Rationale                string
	Source                   string
	InstanceID               string
}

// Common DivergenceKind values. Free-text in the table so future kinds can
// be added without a schema change, but the writer prefers these constants.
const (
	DivergenceKindTextual = "textual"
	DivergenceKindNumeric = "numeric"
	DivergenceKindOrdinal = "ordinal"
	DivergenceKindBinary  = "binary"
	DivergenceKindAbsent  = "absent"
)

// ConflictTracker writes divergence rows to TSDB with per-space rate limiting.
type ConflictTracker struct {
	pool        *pgxpool.Pool
	rateLimit   time.Duration
	lastWritten sync.Map // map[string]time.Time keyed by space_id
}

// NewConflictTracker constructs a tracker. A nil pool yields a no-op tracker
// that returns nil from Track but logs a warn the first time it's called so
// the operator notices the analytics gap.
func NewConflictTracker(pool *pgxpool.Pool, rateLimit time.Duration) *ConflictTracker {
	if rateLimit <= 0 {
		rateLimit = time.Minute // sprint-plan default
	}
	return &ConflictTracker{pool: pool, rateLimit: rateLimit}
}

// Track records one divergence row to `guidance_conflicts`. Returns nil if
// the rate limiter or the nil-pool guard suppressed the write — callers
// should not treat suppression as an error since divergence data is
// statistical, not transactional. Returns a non-nil error only on real
// pgxpool failures so observability picks them up.
func (c *ConflictTracker) Track(ctx context.Context, rec ConflictRecord) error {
	if c == nil {
		return nil
	}
	if rec.SpaceID == "" {
		return fmt.Errorf("conflict_tracker: SpaceID required")
	}
	if rec.DivergenceKind == "" {
		return fmt.Errorf("conflict_tracker: DivergenceKind required")
	}
	if rec.Source == "" {
		return fmt.Errorf("conflict_tracker: Source required")
	}

	// Per-space rate limit. CompareAndSwap-style guard: load → compare → store.
	// Race window between load and store is acceptable — at worst we write
	// two rows in the same second, which is well within table tolerance.
	now := time.Now()
	if last, ok := c.lastWritten.Load(rec.SpaceID); ok {
		if lastT, isTime := last.(time.Time); isTime && now.Sub(lastT) < c.rateLimit {
			return nil
		}
	}
	c.lastWritten.Store(rec.SpaceID, now)

	if c.pool == nil {
		// Fail-open: don't block the caller, but log the gap so operators
		// notice when conflict_tracker is misconfigured.
		slog.Warn("conflict_tracker: nil pool, skipping write",
			"space_id", rec.SpaceID, "source", rec.Source, "kind", rec.DivergenceKind)
		return nil
	}

	conflictID := cuid2.Generate()
	rationale := truncateRationale(rec.Rationale, 1024)
	if rec.ContextHash == "" {
		rec.ContextHash = HashContext(rec.JiminyRecommendation, rec.RSICRecommendation, rec.ConsultingRecommendation)
	}

	const sqlInsert = `INSERT INTO guidance_conflicts
		(time, conflict_id, space_id, context_hash,
		 jiminy_recommendation, rsic_recommendation, consulting_recommendation,
		 divergence_kind, rationale, source, instance_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := c.pool.Exec(ctx, sqlInsert,
		now,
		conflictID,
		rec.SpaceID,
		rec.ContextHash,
		nullIfEmpty(rec.JiminyRecommendation),
		nullIfEmpty(rec.RSICRecommendation),
		nullIfEmpty(rec.ConsultingRecommendation),
		rec.DivergenceKind,
		nullIfEmpty(rationale),
		rec.Source,
		rec.InstanceID,
	)
	if err != nil {
		return fmt.Errorf("conflict_tracker: insert: %w", err)
	}
	return nil
}

// HashContext is a stable 16-hex-char digest of the three recommendations,
// used as the default context_hash when the caller doesn't precompute one
// from richer upstream signal. blake2b-equivalent collision resistance via
// SHA-256 truncation; 16 chars (64 bits) is plenty for analytics joins.
func HashContext(j, r, c string) string {
	h := sha256.New()
	h.Write([]byte(j))
	h.Write([]byte{0})
	h.Write([]byte(r))
	h.Write([]byte{0})
	h.Write([]byte(c))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func truncateRationale(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
