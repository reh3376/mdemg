// Package process implements the JIMINY-PROCESS-OBSERVER-01 (task #160)
// grader — reads `process_events` from TSDB and, for each newly-observed
// terminal event (e.g. git_commit), runs the registered matchers to decide
// whether a rule was followed / missed / incomplete. Writes verdicts to
// `process_outcomes` via the shipped ProcessOutcomesWriter.
//
// Matcher contract:
//   - Fail-open: missing evidence ≠ violation. Only "event present with
//     failed outcome" produces process_incomplete; "no event at all" is
//     process_missed (a signal that the rule wasn't obeyed on the observed
//     path but distinct from an explicit failure).
//   - Idempotent: matchers must be safe to re-run on the same terminal
//     event; the grader loop uses a session+commit fingerprint gate so
//     they typically only run once per event.
//   - Config-gated: every matcher checks its own PROCESS_MATCHER_<RULE>
//     _ENABLED flag; the registry runs only the enabled ones.
package process

import (
	"context"
	"log/slog"
	"time"

	"mdemg/internal/tsdb"
)

// PoolIface is the narrow subset of pgxpool.Pool the grader needs. Kept small
// so tests can supply a fake without importing pgx.
type PoolIface interface {
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
}

// Rows is the narrow scan interface (mirrors pgx.Rows). Adapter in Setup
// wraps the real pgx.Rows.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

// TerminalEvent is one "should this rule have been followed by now?"
// anchor — typically a git_commit or git_push. Matchers look BACKWARDS
// from this event in the same session for evidence.
type TerminalEvent struct {
	EventID   string
	SpaceID   string
	SessionID string
	Time      time.Time
}

// Verdict is a matcher's opinion on one (terminal event, rule) pair.
type Verdict struct {
	ConstraintCode  string
	OutcomeType     string // process_followed | process_missed | process_incomplete
	EvidenceEventID string // empty for process_missed
	Reason          string
}

// Matcher grades a single rule against a terminal event.
type Matcher interface {
	// Name is the matcher's identifier (persisted in process_outcomes.matcher_name).
	Name() string
	// TerminalEventType is the event_type the matcher wants to anchor on.
	// The grader loop routes only events of that type into the matcher.
	TerminalEventType() string
	// ConstraintCode is the Jiminy rule this matcher grades (populates
	// process_outcomes.constraint_code so the aggregator can join back).
	ConstraintCode() string
	// Grade produces a verdict (or returns skip=true when the matcher
	// declines to emit a row for this event — e.g. defensive fail-open on
	// missing prior events).
	Grade(ctx context.Context, pool PoolIface, ev TerminalEvent) (v Verdict, skip bool, err error)
}

// Grader owns the periodic loop + the matcher registry.
type Grader struct {
	pool     PoolIface
	writer   *tsdb.ProcessOutcomesWriter
	matchers []Matcher
	interval time.Duration
	spaceID  string
	// lastGraded tracks the newest event.time we've already processed per
	// (matcher_name, terminal_event_type) so we don't grade the same event
	// twice across ticks. In-memory only; fresh restart replays the last
	// window (bounded by lookbackAtStartup).
	lastGraded         map[string]time.Time
	lookbackAtStartup  time.Duration
	// done is closed on Close(); loop returns.
	done chan struct{}
}

// NewGrader constructs the grader. interval ≤ 0 falls back to 60s.
// spaceID is used to scope the "fresh terminal events" query — pass the
// operator's active space (typically mdemg-dev in production).
func NewGrader(pool PoolIface, writer *tsdb.ProcessOutcomesWriter, spaceID string, interval time.Duration) *Grader {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Grader{
		pool:              pool,
		writer:            writer,
		matchers:          make([]Matcher, 0, 4),
		interval:          interval,
		spaceID:           spaceID,
		lastGraded:        make(map[string]time.Time),
		lookbackAtStartup: 24 * time.Hour,
		done:              make(chan struct{}),
	}
}

// Register adds a matcher. Not thread-safe; call before Run.
func (g *Grader) Register(m Matcher) {
	if m == nil {
		return
	}
	g.matchers = append(g.matchers, m)
}

// MatcherCount is a diagnostic accessor for smoke logs.
func (g *Grader) MatcherCount() int {
	return len(g.matchers)
}

// Run is the supervised worker function (SUPERVISOR-002 contract):
// blocking `func(ctx) error`; nil return = intentional completion (no
// restart); panic/error = supervised restart. Returns nil when ctx is
// cancelled or Close() fires — both are graceful shutdown paths.
func (g *Grader) Run(ctx context.Context) error {
	if g == nil {
		return nil
	}
	if len(g.matchers) == 0 {
		slog.Info("process grader: no matchers registered — sleeping until shutdown")
		<-ctx.Done()
		return nil
	}
	slog.Info("process grader: started",
		"interval", g.interval,
		"matcher_count", len(g.matchers),
		"space_id", g.spaceID)
	// Seed lastGraded at startup to (now - lookbackAtStartup) so a fresh
	// process replays the last 24h once, then only grades new events.
	seedFrom := time.Now().Add(-g.lookbackAtStartup)
	for _, m := range g.matchers {
		key := terminalKey(m)
		g.lastGraded[key] = seedFrom
	}
	tick := time.NewTicker(g.interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-g.done:
			return nil
		case <-tick.C:
			g.tickOnce(ctx)
		}
	}
}

// Close asks Run to return. Safe to call multiple times.
func (g *Grader) Close() {
	if g == nil {
		return
	}
	select {
	case <-g.done:
	default:
		close(g.done)
	}
}

func (g *Grader) tickOnce(ctx context.Context) {
	for _, m := range g.matchers {
		key := terminalKey(m)
		since := g.lastGraded[key]
		terminals, newest, err := g.fetchTerminalEvents(ctx, m.TerminalEventType(), since)
		if err != nil {
			slog.Warn("process grader: fetch terminals failed",
				"matcher", m.Name(),
				"since", since,
				"error", err)
			continue
		}
		if len(terminals) == 0 {
			continue
		}
		for _, ev := range terminals {
			verdict, skip, err := m.Grade(ctx, g.pool, ev)
			if err != nil {
				slog.Warn("process grader: matcher error",
					"matcher", m.Name(),
					"event_id", ev.EventID,
					"error", err)
				continue
			}
			if skip {
				continue
			}
			g.writer.Record(tsdb.ProcessOutcomeRow{
				SpaceID:         ev.SpaceID,
				SessionID:       ev.SessionID,
				ConstraintCode:  verdict.ConstraintCode,
				OutcomeType:     verdict.OutcomeType,
				EvidenceEventID: verdict.EvidenceEventID,
				Reason:          verdict.Reason,
				MatcherName:     m.Name(),
				Time:            ev.Time,
			})
		}
		if newest.After(since) {
			g.lastGraded[key] = newest
		}
	}
}

// fetchTerminalEvents queries process_events for events of the given
// terminal type in the space, newer than `since`. Returns the events + the
// newest event's time (so the grader can advance lastGraded).
func (g *Grader) fetchTerminalEvents(ctx context.Context, eventType string, since time.Time) ([]TerminalEvent, time.Time, error) {
	rows, err := g.pool.Query(ctx, `
		SELECT event_id, space_id, session_id, time
		FROM process_events
		WHERE space_id = $1 AND event_type = $2 AND time > $3
		ORDER BY time ASC
		LIMIT 500
	`, g.spaceID, eventType, since)
	if err != nil {
		return nil, since, err
	}
	defer rows.Close()
	out := make([]TerminalEvent, 0, 32)
	newest := since
	for rows.Next() {
		var ev TerminalEvent
		if err := rows.Scan(&ev.EventID, &ev.SpaceID, &ev.SessionID, &ev.Time); err != nil {
			return nil, since, err
		}
		out = append(out, ev)
		if ev.Time.After(newest) {
			newest = ev.Time
		}
	}
	return out, newest, rows.Err()
}

func terminalKey(m Matcher) string {
	return m.Name() + "|" + m.TerminalEventType()
}
