// Sprint HITL-REVIEW-001 Epic 1 — review_grades writer (V0028).
//
// Buffered + CopyFrom for the human-paced gold-grade + reversal-audit stream
// (V0022 reinforcement-writer pattern: bounded FIFO buffer, drop counter,
// registerWriterStats). Plus SYNCHRONOUS point reads (LatestGradeForItem,
// GradeByID) that flush-then-query so idempotency + reversal get read-your-
// writes — a buffered-only read could miss an un-flushed grade and double-apply.
//
// The row carries gold_dimensions / reinforcement_detail as pre-marshalled JSON
// strings so internal/tsdb stays free of internal/review (the api layer
// marshals the review types).
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

// reviewPoolIface is poolIface plus the point read/exec the idempotency +
// reversal paths need. *pgxpool.Pool satisfies it.
type reviewPoolIface interface {
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ReviewGradeRow mirrors the V0028 review_grades columns. JSON fields are
// pre-marshalled; "" → NULL for the nullable ones.
type ReviewGradeRow struct {
	Time                    time.Time
	GradeID                 string
	DatasetID               string
	ItemID                  string
	SpaceID                 string
	GoldScore               float64
	GoldDimensionsJSON      string // jsonb (NOT NULL)
	GraderID                string
	RubricVersion           string
	GradedAt                time.Time
	ReinforcementApplied    bool
	ReinforcementDetailJSON string // jsonb; "" → NULL
	Reversed                bool
	ReversesGradeID         string // "" → NULL
	InstanceID              string
	SuggestedGuidance       string // SME corrective example (NOT NULL DEFAULT '')
}

// ReviewGradesWriter buffers review_grades rows and flushes via CopyFrom.
type ReviewGradesWriter struct {
	pool          reviewPoolIface
	buffer        []ReviewGradeRow
	maxBufferSize int
	mu            sync.Mutex
	flushTick     *time.Ticker
	done          chan struct{}
	flushSuccess  atomic.Int64
	flushFailure  atomic.Int64
	flushRows     atomic.Int64
	droppedRows   atomic.Int64
}

// NewReviewGradesWriter constructs a buffered writer. flushInterval ≤ 0 → 15s;
// maxBufferSize ≤ 0 → unlimited.
func NewReviewGradesWriter(pool reviewPoolIface, flushInterval time.Duration, maxBufferSize int) *ReviewGradesWriter {
	if flushInterval <= 0 {
		flushInterval = 15 * time.Second
	}
	w := &ReviewGradesWriter{
		pool:          pool,
		buffer:        make([]ReviewGradeRow, 0, 16),
		maxBufferSize: maxBufferSize,
		done:          make(chan struct{}),
	}
	registerWriterStats("review_grades", func() FlushStats {
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

func (w *ReviewGradesWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("review_grades: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record buffers one row (FIFO eviction at cap). Non-blocking.
func (w *ReviewGradesWriter) Record(row ReviewGradeRow) {
	if row.Time.IsZero() {
		row.Time = time.Now()
	}
	if row.GradedAt.IsZero() {
		row.GradedAt = row.Time
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
func (w *ReviewGradesWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]ReviewGradeRow, 0, 16)
	w.mu.Unlock()

	rows := make([][]any, 0, len(batch))
	for _, r := range batch {
		rows = append(rows, []any{
			r.Time, r.GradeID, r.DatasetID, r.ItemID, r.SpaceID,
			r.GoldScore, r.GoldDimensionsJSON, r.GraderID, r.RubricVersion,
			r.GradedAt, r.ReinforcementApplied,
			nullableString(r.ReinforcementDetailJSON),
			r.Reversed, nullableString(r.ReversesGradeID),
			nullableString(r.InstanceID), r.SuggestedGuidance,
		})
	}
	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"review_grades"},
		[]string{
			"time", "grade_id", "dataset_id", "item_id", "space_id",
			"gold_score", "gold_dimensions", "grader_id", "rubric_version",
			"graded_at", "reinforcement_applied", "reinforcement_detail",
			"reversed", "reverses_grade_id", "instance_id", "suggested_guidance",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("review_grades: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
		return err
	}
	slog.Debug("review_grades: flushed", "count", len(batch))
	w.flushSuccess.Add(1)
	w.flushRows.Add(int64(len(batch)))
	return nil
}

// LatestGradeForItem returns the most recent NON-reversed grade for
// (datasetID,itemID), or (nil,nil) if none. Flushes first for read-your-writes.
func (w *ReviewGradesWriter) LatestGradeForItem(ctx context.Context, datasetID, itemID string) (*ReviewGradeRow, error) {
	if err := w.Flush(ctx); err != nil {
		return nil, err
	}
	row := w.pool.QueryRow(ctx, `
		SELECT grade_id, dataset_id, item_id, space_id, gold_score,
		       COALESCE(gold_dimensions::text,''), grader_id, rubric_version,
		       reinforcement_applied, COALESCE(reinforcement_detail::text,''),
		       reversed, time
		FROM review_grades
		WHERE dataset_id = $1 AND item_id = $2 AND reversed = FALSE
		ORDER BY time DESC LIMIT 1`, datasetID, itemID)
	return scanGradeRow(row)
}

// GradeByID returns the grade with the given id (any state), or (nil,nil).
func (w *ReviewGradesWriter) GradeByID(ctx context.Context, gradeID string) (*ReviewGradeRow, error) {
	if err := w.Flush(ctx); err != nil {
		return nil, err
	}
	row := w.pool.QueryRow(ctx, `
		SELECT grade_id, dataset_id, item_id, space_id, gold_score,
		       COALESCE(gold_dimensions::text,''), grader_id, rubric_version,
		       reinforcement_applied, COALESCE(reinforcement_detail::text,''),
		       reversed, time
		FROM review_grades
		WHERE grade_id = $1
		ORDER BY time DESC LIMIT 1`, gradeID)
	return scanGradeRow(row)
}

// MarkReversed sets reversed=true on the original grade (the reversal row is a
// separate Record). Idempotent.
func (w *ReviewGradesWriter) MarkReversed(ctx context.Context, gradeID string) error {
	if err := w.Flush(ctx); err != nil {
		return err
	}
	_, err := w.pool.Exec(ctx, `UPDATE review_grades SET reversed = TRUE WHERE grade_id = $1`, gradeID)
	return err
}

func scanGradeRow(row pgx.Row) (*ReviewGradeRow, error) {
	var r ReviewGradeRow
	err := row.Scan(&r.GradeID, &r.DatasetID, &r.ItemID, &r.SpaceID, &r.GoldScore,
		&r.GoldDimensionsJSON, &r.GraderID, &r.RubricVersion,
		&r.ReinforcementApplied, &r.ReinforcementDetailJSON, &r.Reversed, &r.Time)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// Stats returns flush + drop counters.
func (w *ReviewGradesWriter) Stats() FlushStats {
	return FlushStats{
		SuccessCount:  w.flushSuccess.Load(),
		FailureCount:  w.flushFailure.Load(),
		TotalRows:     w.flushRows.Load(),
		OverflowCount: w.droppedRows.Load(),
	}
}

// Close stops the flush loop and drains the buffer.
func (w *ReviewGradesWriter) Close() {
	w.flushTick.Stop()
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("review_grades: final flush failed", "error", err)
	}
}
