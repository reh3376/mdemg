// Sprint MODEL-DIST-001 — model_install_events writer (V0021).
//
// One row per `mdemg model pull|verify|remove` invocation. Unlike the
// retrieval-side V0017/V0018/V0019/V0020 writers, this is invoked from the
// short-lived CLI process — so writes are synchronous single-row INSERTs,
// not buffered + CopyFrom. Fire-and-forget semantics: if TSDB is unreachable,
// the CLI command logs a warning and continues; the operation is observability,
// not correctness-critical.
package tsdb

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nrednav/cuid2"
)

// modelInstallPool is the minimal Pool interface needed to write a single
// row via INSERT. The package-level poolIface declares CopyFrom (used by
// buffered writers); we add an Exec-shaped contract here so the model
// install writer's single-row pattern works without touching the
// buffered-writer interface.
type modelInstallPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// ModelInstallEventRow mirrors the V0021 model_install_events columns. All
// strings are pre-truncated by the caller as needed; this writer doesn't
// re-validate.
type ModelInstallEventRow struct {
	EventType   string // "pull" | "verify" | "remove"
	BackendName string
	Namespace   string
	ModelName   string
	Quant       string
	Adapter     bool
	Success     bool
	LatencyMS   int64
	SHA256      string
	SizeBytes   int64
	ErrMessage  string
}

// errMessageMaxLen bounds the err_message column at write time. PostgreSQL
// TEXT has no in-table limit, but huge error blobs aren't useful for
// retrospect and bloat the index.
const errMessageMaxLen = 1024

// RecordModelInstallEvent inserts one row synchronously. Callers should
// invoke from a goroutine with a short timeout if they want fire-and-forget
// semantics — this function itself blocks until the INSERT completes (or
// errors).
//
// The pool argument must satisfy modelInstallPool (a pgxpool.Pool does).
// Pass nil to silently no-op (useful when TSDB is disabled in config).
func RecordModelInstallEvent(ctx context.Context, pool modelInstallPool, row ModelInstallEventRow) error {
	if pool == nil {
		return nil
	}
	if len(row.ErrMessage) > errMessageMaxLen {
		row.ErrMessage = row.ErrMessage[:errMessageMaxLen]
	}

	// Optional fields → NULL when unset.
	var quant any
	if row.Quant != "" {
		quant = row.Quant
	}
	var latency any
	if row.LatencyMS > 0 {
		latency = row.LatencyMS
	}
	var sha any
	if row.SHA256 != "" {
		sha = row.SHA256
	}
	var size any
	if row.SizeBytes > 0 {
		size = row.SizeBytes
	}
	var errMsg any
	if row.ErrMessage != "" {
		errMsg = row.ErrMessage
	}

	const insertSQL = `
		INSERT INTO model_install_events (
			event_id, recorded_at, event_type, backend_name,
			namespace, model_name, quant, adapter,
			success, latency_ms, sha256, size_bytes, err_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := pool.Exec(ctx, insertSQL,
		cuid2.Generate(),
		time.Now(),
		row.EventType,
		row.BackendName,
		row.Namespace,
		row.ModelName,
		quant,
		row.Adapter,
		row.Success,
		latency,
		sha,
		size,
		errMsg,
	)
	if err != nil {
		slog.Warn("model_install_events: insert failed", "error", err, "event_type", row.EventType)
		return err
	}
	return nil
}

