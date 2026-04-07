package alert

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ruleState tracks per-rule evaluation state for ForDuration logic.
type ruleState struct {
	conditionFirstTrue time.Time // when the condition first became true
	fired              bool      // whether we've already fired for this window
	lastEval           time.Time // last evaluation time
}

// Evaluator runs alert rules on a periodic schedule against TSDB.
type Evaluator struct {
	mu         sync.Mutex
	rules      []AlertRule
	pool       *pgxpool.Pool
	dispatcher *Dispatcher
	state      map[string]*ruleState
	interval   time.Duration
	stopCh     chan struct{}
}

// NewEvaluator creates an alert evaluator with the given rules, TSDB pool, and dispatcher.
func NewEvaluator(rules []AlertRule, pool *pgxpool.Pool, dispatcher *Dispatcher, interval time.Duration) *Evaluator {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	state := make(map[string]*ruleState, len(rules))
	for _, r := range rules {
		state[r.ID] = &ruleState{}
	}
	return &Evaluator{
		rules:      rules,
		pool:       pool,
		dispatcher: dispatcher,
		state:      state,
		interval:   interval,
		stopCh:     make(chan struct{}),
	}
}

// Start begins the periodic evaluation loop. It blocks until Stop is called.
func (e *Evaluator) Start() {
	slog.Info("alert evaluator started", "rules", len(e.rules), "interval", e.interval)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.evaluateAll()
		case <-e.stopCh:
			slog.Info("alert evaluator stopped")
			return
		}
	}
}

// Stop signals the evaluator to stop.
func (e *Evaluator) Stop() {
	close(e.stopCh)
}

// evaluateAll evaluates all enabled rules whose interval has elapsed.
func (e *Evaluator) evaluateAll() {
	now := time.Now()
	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}

		e.mu.Lock()
		st := e.state[rule.ID]
		if st == nil {
			st = &ruleState{}
			e.state[rule.ID] = st
		}

		// Skip if rule's individual interval hasn't elapsed
		if !st.lastEval.IsZero() && now.Sub(st.lastEval) < rule.Interval {
			e.mu.Unlock()
			continue
		}
		st.lastEval = now
		e.mu.Unlock()

		e.evaluateRule(rule, now)
	}
}

// evaluateRule queries TSDB and evaluates a single rule.
func (e *Evaluator) evaluateRule(rule AlertRule, now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var value float64
	err := e.pool.QueryRow(ctx, rule.QuerySQL).Scan(&value)
	if err != nil {
		// TSDB unavailable or no data — reset state and skip
		slog.Debug("alert evaluator: query failed",
			"rule", rule.ID, "error", err)
		e.mu.Lock()
		st := e.state[rule.ID]
		st.conditionFirstTrue = time.Time{}
		st.fired = false
		e.mu.Unlock()
		return
	}

	breached := checkThreshold(value, rule.Threshold, rule.Operator)

	e.mu.Lock()
	defer e.mu.Unlock()

	st := e.state[rule.ID]
	if !breached {
		// Condition cleared — reset state
		st.conditionFirstTrue = time.Time{}
		st.fired = false
		return
	}

	// Condition is true
	if st.conditionFirstTrue.IsZero() {
		st.conditionFirstTrue = now
	}

	// Check if ForDuration has elapsed
	if rule.ForDuration > 0 && now.Sub(st.conditionFirstTrue) < rule.ForDuration {
		return // not yet sustained long enough
	}

	// Fire if not already fired for this window
	if !st.fired {
		st.fired = true
		go e.dispatcher.Send(context.Background(), Alert{
			Service:  rule.Service,
			Severity: rule.Severity,
			Title:    rule.Title,
			Message:  rule.Title,
		})
	}
}

// checkThreshold compares a value against a threshold using the given operator.
func checkThreshold(value, threshold float64, op string) bool {
	if math.IsNaN(value) {
		return false
	}
	switch op {
	case "gt":
		return value > threshold
	case "lt":
		return value < threshold
	case "gte":
		return value >= threshold
	case "lte":
		return value <= threshold
	default:
		return false
	}
}
