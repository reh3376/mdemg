// Sprint JIMINY-CONTRADICTED-BRIDGE-001 Epic 1 — contradicted_correction_drafts
// writer (V0030).
//
// Buffered + CopyFrom for the low-volume Jiminy contradicted-outcome bridge
// (baseline ~3/mo lifetime, currently). Plus synchronous point-reads for the
// HITL surface (FetchCandidates, FetchByID) and synchronous status transitions
// (MarkApproved / MarkDismissed / ResetToPending) — flush-then-query for
// read-your-writes on the approve path.
//
// The row shape and the writer live in internal/tsdb so the HITL package can
// query it without any tsdb→jiminy or tsdb→review import cycle.
package tsdb

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// contradictedPoolIface is poolIface plus the point read/update the HITL
// dataset + sink need. *pgxpool.Pool satisfies it.
type contradictedPoolIface interface {
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ContradictedDraftRow mirrors the V0030 contradicted_correction_drafts
// columns. Status is "" → "pending" at write time.
type ContradictedDraftRow struct {
	Time            time.Time
	ID              string // CUIDv2
	SpaceID         string
	GuidanceID      string
	GuidanceType    string
	SourceNodeID    string
	GuidanceContent string
	ActionSummary   string
	Similarity      float64
	ActionHash      string // sha256(guidance_id||normalize(action))[:16]
	DraftIncorrect  string
	DraftCorrect    string
	Status          string // pending | approved | dismissed
	SessionID       string
	InstanceID      string
	AppliedAt       *time.Time // set on approve
	AppliedObsID    string     // set on approve
}

// ContradictedDraftsWriter buffers draft rows and flushes via CopyFrom.
type ContradictedDraftsWriter struct {
	pool          contradictedPoolIface
	buffer        []ContradictedDraftRow
	maxBufferSize int
	mu            sync.Mutex
	flushTick     *time.Ticker
	done          chan struct{}
	flushSuccess  atomic.Int64
	flushFailure  atomic.Int64
	flushRows     atomic.Int64
	droppedRows   atomic.Int64
}

// NewContradictedDraftsWriter constructs a buffered writer. flushInterval ≤ 0
// → 30s; maxBufferSize ≤ 0 → unlimited.
func NewContradictedDraftsWriter(pool contradictedPoolIface, flushInterval time.Duration, maxBufferSize int) *ContradictedDraftsWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &ContradictedDraftsWriter{
		pool:          pool,
		buffer:        make([]ContradictedDraftRow, 0, 16),
		maxBufferSize: maxBufferSize,
		done:          make(chan struct{}),
	}
	registerWriterStats("contradicted_drafts", func() FlushStats {
		return FlushStats{
			SuccessCount:  w.flushSuccess.Load(),
			FailureCount:  w.flushFailure.Load(),
			TotalRows:     w.flushRows.Load(),
			OverflowCount: w.droppedRows.Load(),
		}
	})
	w.flushTick = time.NewTicker(flushInterval)
	go w.flushLoop()
	return w
}

func (w *ContradictedDraftsWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("contradicted_drafts: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// RecordDraft is the jiminy-interface entry point — takes primitives so the
// jiminy package can call it without importing tsdb (avoids import cycle).
// The bridge hook in jiminy/service.go invokes this on OutcomeContradicted.
func (w *ContradictedDraftsWriter) RecordDraft(
	id, spaceID, guidanceID, guidanceType, sourceNodeID,
	guidanceContent, actionSummary, actionHash,
	draftIncorrect, draftCorrect, sessionID string,
	similarity float64,
) {
	w.Record(ContradictedDraftRow{
		ID:              id,
		SpaceID:         spaceID,
		GuidanceID:      guidanceID,
		GuidanceType:    guidanceType,
		SourceNodeID:    sourceNodeID,
		GuidanceContent: guidanceContent,
		ActionSummary:   actionSummary,
		Similarity:      similarity,
		ActionHash:      actionHash,
		DraftIncorrect:  draftIncorrect,
		DraftCorrect:    draftCorrect,
		SessionID:       sessionID,
	})
}

// Record buffers one row (FIFO eviction at cap). Non-blocking.
func (w *ContradictedDraftsWriter) Record(row ContradictedDraftRow) {
	if row.Time.IsZero() {
		row.Time = time.Now()
	}
	if row.Status == "" {
		row.Status = "pending"
	}
	w.mu.Lock()
	if w.maxBufferSize > 0 && len(w.buffer) >= w.maxBufferSize {
		evict := len(w.buffer) - w.maxBufferSize + 1
		w.buffer = w.buffer[evict:]
		w.droppedRows.Add(int64(evict))
	}
	w.buffer = append(w.buffer, row)
	w.mu.Unlock()
}

// Flush writes buffered rows via CopyFrom and clears the buffer.
func (w *ContradictedDraftsWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]ContradictedDraftRow, 0, 16)
	w.mu.Unlock()

	rows := make([][]any, 0, len(batch))
	for _, r := range batch {
		var appliedAt any
		if r.AppliedAt != nil {
			appliedAt = *r.AppliedAt
		}
		rows = append(rows, []any{
			r.Time, r.ID, r.SpaceID, r.GuidanceID, r.GuidanceType,
			r.SourceNodeID, r.GuidanceContent, r.ActionSummary,
			r.Similarity, r.ActionHash, r.DraftIncorrect, r.DraftCorrect,
			r.Status, r.SessionID, appliedAt,
			nullableString(r.AppliedObsID), nullableString(r.InstanceID),
		})
	}
	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"contradicted_correction_drafts"},
		[]string{
			"time", "id", "space_id", "guidance_id", "guidance_type",
			"source_node_id", "guidance_content", "action_summary",
			"similarity", "action_hash", "draft_incorrect", "draft_correct",
			"status", "session_id", "applied_at",
			"applied_obs_id", "instance_id",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("contradicted_drafts: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
		return err
	}
	slog.Debug("contradicted_drafts: flushed", "count", len(batch))
	w.flushSuccess.Add(1)
	w.flushRows.Add(int64(len(batch)))
	return nil
}

// FetchPendingBySpace returns up to `limit` pending drafts for the given
// space, newest first. Flushes first for read-your-writes on freshly-emitted
// drafts.
func (w *ContradictedDraftsWriter) FetchPendingBySpace(ctx context.Context, spaceID string, limit int) ([]ContradictedDraftRow, error) {
	if err := w.Flush(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := w.pool.Query(ctx, `
		SELECT time, id, space_id, guidance_id, guidance_type,
		       source_node_id, guidance_content, action_summary,
		       similarity, action_hash, draft_incorrect, draft_correct,
		       status, session_id, applied_at,
		       COALESCE(applied_obs_id, ''), COALESCE(instance_id, '')
		FROM contradicted_correction_drafts
		WHERE space_id = $1 AND status = 'pending'
		ORDER BY time DESC LIMIT $2`, spaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ContradictedDraftRow, 0, limit)
	for rows.Next() {
		var r ContradictedDraftRow
		var appliedAt *time.Time
		if err := rows.Scan(&r.Time, &r.ID, &r.SpaceID, &r.GuidanceID, &r.GuidanceType,
			&r.SourceNodeID, &r.GuidanceContent, &r.ActionSummary,
			&r.Similarity, &r.ActionHash, &r.DraftIncorrect, &r.DraftCorrect,
			&r.Status, &r.SessionID, &appliedAt,
			&r.AppliedObsID, &r.InstanceID); err != nil {
			return nil, err
		}
		r.AppliedAt = appliedAt
		out = append(out, r)
	}
	return out, rows.Err()
}

// FetchByID returns the draft with the given id (any status), or (nil,nil).
// Flushes first for read-your-writes.
func (w *ContradictedDraftsWriter) FetchByID(ctx context.Context, id string) (*ContradictedDraftRow, error) {
	if err := w.Flush(ctx); err != nil {
		return nil, err
	}
	row := w.pool.QueryRow(ctx, `
		SELECT time, id, space_id, guidance_id, guidance_type,
		       source_node_id, guidance_content, action_summary,
		       similarity, action_hash, draft_incorrect, draft_correct,
		       status, session_id, applied_at,
		       COALESCE(applied_obs_id, ''), COALESCE(instance_id, '')
		FROM contradicted_correction_drafts
		WHERE id = $1
		ORDER BY time DESC LIMIT 1`, id)
	var r ContradictedDraftRow
	var appliedAt *time.Time
	err := row.Scan(&r.Time, &r.ID, &r.SpaceID, &r.GuidanceID, &r.GuidanceType,
		&r.SourceNodeID, &r.GuidanceContent, &r.ActionSummary,
		&r.Similarity, &r.ActionHash, &r.DraftIncorrect, &r.DraftCorrect,
		&r.Status, &r.SessionID, &appliedAt,
		&r.AppliedObsID, &r.InstanceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	r.AppliedAt = appliedAt
	return &r, nil
}

// DedupExists returns true iff a NON-dismissed draft with the given
// (guidance_id, action_hash) already exists in the last 7 days. Used by the
// bridge hook to skip duplicate emission on repeat violations. Flush-first
// for read-your-writes across a single-process race window.
func (w *ContradictedDraftsWriter) DedupExists(ctx context.Context, guidanceID, actionHash string) (bool, error) {
	if actionHash == "" {
		return false, nil
	}
	if err := w.Flush(ctx); err != nil {
		return false, err
	}
	var exists bool
	err := w.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM contradicted_correction_drafts
			WHERE guidance_id = $1 AND action_hash = $2
			  AND status != 'dismissed'
			  AND time > NOW() - INTERVAL '7 days'
		)`, guidanceID, actionHash).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// MarkApproved transitions a draft to status='approved' and records the L0
// obs id the sink created. Idempotent.
func (w *ContradictedDraftsWriter) MarkApproved(ctx context.Context, id string, appliedObsID string) error {
	if err := w.Flush(ctx); err != nil {
		return err
	}
	_, err := w.pool.Exec(ctx, `
		UPDATE contradicted_correction_drafts
		SET status = 'approved', applied_at = NOW(), applied_obs_id = $2
		WHERE id = $1`, id, appliedObsID)
	return err
}

// MarkDismissed transitions a draft to status='dismissed'. Idempotent.
func (w *ContradictedDraftsWriter) MarkDismissed(ctx context.Context, id string) error {
	if err := w.Flush(ctx); err != nil {
		return err
	}
	_, err := w.pool.Exec(ctx, `
		UPDATE contradicted_correction_drafts SET status = 'dismissed' WHERE id = $1`, id)
	return err
}

// ResetToPending flips a draft back to pending (used by Sink.Reverse).
// Idempotent; does NOT clear applied_obs_id (the L0 obs it created stays).
func (w *ContradictedDraftsWriter) ResetToPending(ctx context.Context, id string) error {
	if err := w.Flush(ctx); err != nil {
		return err
	}
	_, err := w.pool.Exec(ctx, `
		UPDATE contradicted_correction_drafts SET status = 'pending' WHERE id = $1`, id)
	return err
}

// Stats returns flush + drop counters.
func (w *ContradictedDraftsWriter) Stats() FlushStats {
	return FlushStats{
		SuccessCount:  w.flushSuccess.Load(),
		FailureCount:  w.flushFailure.Load(),
		TotalRows:     w.flushRows.Load(),
		OverflowCount: w.droppedRows.Load(),
	}
}

// Close stops the flush loop and drains the buffer.
func (w *ContradictedDraftsWriter) Close() {
	w.flushTick.Stop()
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("contradicted_drafts: final flush failed", "error", err)
	}
}
