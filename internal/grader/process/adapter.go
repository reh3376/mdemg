// PoolAdapter bridges a real pgxpool.Pool to the narrow PoolIface the grader
// uses. Kept in this package so tests can supply an in-memory fake and the
// real wiring lives in one place (mirrors similar adapter patterns in
// internal/ftloop and internal/eventgraph).
package process

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolAdapter wraps a *pgxpool.Pool. The zero value is not useful — always
// construct with `procgrader.PoolAdapter{Pool: client.Pool()}`.
type PoolAdapter struct {
	Pool *pgxpool.Pool
}

// Query runs a query and returns a Rows adapter.
func (a PoolAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := a.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgxRowsAdapter{rows: rows}, nil
}

// pgxRowsAdapter turns a pgx.Rows into a process.Rows. Owns Close().
type pgxRowsAdapter struct {
	rows pgx.Rows
}

func (r pgxRowsAdapter) Next() bool                     { return r.rows.Next() }
func (r pgxRowsAdapter) Scan(dest ...any) error         { return r.rows.Scan(dest...) }
func (r pgxRowsAdapter) Close()                         { r.rows.Close() }
func (r pgxRowsAdapter) Err() error                     { return r.rows.Err() }
