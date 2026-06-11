package alert

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ruleState tracks per-rule evaluation state for ForDuration logic.
type ruleState struct {
	conditionFirstTrue  time.Time // when the condition first became true
	fired               bool      // whether we've already fired for this window
	lastEval            time.Time // last evaluation time
	consecutiveFailures int       // SUPERVISOR-002: query failures in a row
	failureAlerted      bool      // SUPERVISOR-002: meta-alert sent for this failure streak
	streakStart         time.Time // SUPERVISOR-002: when the current failure streak began
}

// defaultRuleFailureThreshold is the consecutive-query-failure count at which
// the rule-health meta-alert fires (override via SetRuleFailureThreshold /
// ALERT_RULE_FAILURE_THRESHOLD).
const defaultRuleFailureThreshold = 3

// Evaluator runs alert rules on a periodic schedule against TSDB.
type Evaluator struct {
	mu               sync.Mutex
	rules            []AlertRule
	pool             *pgxpool.Pool
	dispatcher       *Dispatcher
	state            map[string]*ruleState
	interval         time.Duration
	stopCh           chan struct{}
	failureThreshold int
	lastAnySuccess   time.Time // SUPERVISOR-002: last successful query across all rules
	globalAlerted    bool      // SUPERVISOR-002: evaluator-degraded alert sent for this outage
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
		rules:            rules,
		pool:             pool,
		dispatcher:       dispatcher,
		state:            state,
		interval:         interval,
		stopCh:           make(chan struct{}),
		failureThreshold: defaultRuleFailureThreshold,
	}
}

// SetRuleFailureThreshold overrides the consecutive-failure count at which a
// rule-health meta-alert fires (SUPERVISOR-002). Non-positive keeps default.
func (e *Evaluator) SetRuleFailureThreshold(n int) {
	if n > 0 {
		e.failureThreshold = n
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
		// SUPERVISOR-002: a rule whose SQL errors is a silently-disabled
		// alert. Warn every time, and fire a rule-health meta-alert after
		// N consecutive failures (directly via the dispatcher — the
		// meta-channel must not depend on the failing rule mechanism).
		slog.Warn("alert evaluator: query failed",
			"rule", rule.ID, "error", err)
		verdict, failures := e.recordRuleFailure(rule.ID, now)
		if e.dispatcher == nil {
			return
		}
		switch verdict {
		case metaFirePerRule:
			go e.dispatcher.Send(context.Background(), Alert{
				// Distinct Service per rule: the dispatcher cooldown key is
				// (Service, Severity) — shared labels suppress each other.
				Service:  "rule-health-" + rule.ID,
				Severity: SeverityHigh,
				Title:    "Alert rule query failing: " + rule.ID,
				Message: fmt.Sprintf("rule %q failed %d consecutive evaluations and is effectively disabled; last error: %v",
					rule.ID, failures, err),
			})
		case metaFireGlobal:
			// No rule is succeeding — TSDB-level outage or schema-wide
			// break. One aggregate alert instead of a per-rule storm.
			go e.dispatcher.Send(context.Background(), Alert{
				Service:  "alert-evaluator-degraded",
				Severity: SeverityHigh,
				Title:    "Alert evaluator degraded: no rule queries succeeding",
				Message: fmt.Sprintf("all rule queries failing (alerting is effectively disabled); first rule at threshold: %q, last error: %v",
					rule.ID, err),
			})
		case metaSilent:
		}
		return
	}

	breached := checkThreshold(value, rule.Threshold, rule.Operator)

	e.mu.Lock()
	defer e.mu.Unlock()

	st := e.state[rule.ID]
	// SUPERVISOR-002: query succeeded — re-arm the rule-health meta-alerts.
	e.recordRuleSuccess(st, now)
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

// metaVerdict is the outcome of recordRuleFailure (SUPERVISOR-002).
type metaVerdict int

const (
	metaSilent      metaVerdict = iota // below threshold or already alerted
	metaFirePerRule                    // this rule is failing while others succeed
	metaFireGlobal                     // no rule is succeeding — aggregate outage alert
)

// recordRuleFailure tracks a rule query failure (SUPERVISOR-002): resets the
// ForDuration state and increments the consecutive-failure streak. At the
// threshold it returns metaFirePerRule when some OTHER rule has succeeded
// since this rule's streak began (broken SQL on this rule while the rest of
// the evaluator is healthy), or metaFireGlobal — once per outage — when
// nothing has succeeded since the streak began (TSDB-level outage; a
// per-rule alert per rule would be a storm duplicating the health prober's
// signal). The discriminator is streak-relative, not wall-clock-relative:
// at outage ONSET lastAnySuccess is always recent, so a freshness window
// would misclassify the first rules to reach threshold (caught live in the
// Epic 5 TSDB-stop drill — 2 per-rule alerts leaked before the aggregate).
func (e *Evaluator) recordRuleFailure(ruleID string, now time.Time) (metaVerdict, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.state[ruleID]
	if st == nil {
		st = &ruleState{}
		e.state[ruleID] = st
	}
	st.conditionFirstTrue = time.Time{}
	st.fired = false
	if st.consecutiveFailures == 0 {
		st.streakStart = now
	}
	st.consecutiveFailures++
	if st.consecutiveFailures < e.failureThreshold || st.failureAlerted {
		return metaSilent, st.consecutiveFailures
	}
	st.failureAlerted = true

	if e.lastAnySuccess.After(st.streakStart) {
		return metaFirePerRule, st.consecutiveFailures
	}
	if e.globalAlerted {
		return metaSilent, st.consecutiveFailures
	}
	e.globalAlerted = true
	return metaFireGlobal, st.consecutiveFailures
}

// recordRuleSuccess re-arms the rule-health meta-alerts after a successful
// query (SUPERVISOR-002).
func (e *Evaluator) recordRuleSuccess(st *ruleState, now time.Time) {
	st.consecutiveFailures = 0
	st.failureAlerted = false
	e.lastAnySuccess = now
	e.globalAlerted = false
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
