package jiminy

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// CodeComprehensionTracker tracks comprehension scores per constraint code
// using a sliding window. When a code's average comprehension drops below
// a threshold over N samples, it signals that the code should be regenerated.
type CodeComprehensionTracker struct {
	mu          sync.Mutex
	minSamples  int
	threshold   float64
	maxPerHour  int
	cooldown    time.Duration
	scores      map[string]*codeScoreWindow // constraint_code → window
	regenLog    []time.Time                 // timestamps of recent regenerations
	codeGen     *ConstraintCodeGenerator
}

type codeScoreWindow struct {
	scores     []float64
	lastRegen  time.Time
}

// NewCodeComprehensionTracker creates a new tracker.
// threshold: avg comprehension below this triggers regen (e.g., 0.3)
// minSamples: minimum scores before evaluating (e.g., 10)
// maxPerHour: max regenerations per hour to prevent loops (e.g., 5)
// cooldown: minimum time between regens for the same code (e.g., 1 hour)
func NewCodeComprehensionTracker(
	threshold float64,
	minSamples int,
	maxPerHour int,
	cooldown time.Duration,
	codeGen *ConstraintCodeGenerator,
) *CodeComprehensionTracker {
	return &CodeComprehensionTracker{
		threshold:  threshold,
		minSamples: minSamples,
		maxPerHour: maxPerHour,
		cooldown:   cooldown,
		scores:     make(map[string]*codeScoreWindow),
		codeGen:    codeGen,
	}
}

// Record adds a comprehension score for a constraint code.
// Returns true if the code needs regeneration.
func (t *CodeComprehensionTracker) Record(constraintCode string, comprehension float64) bool {
	if constraintCode == "" {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	w, ok := t.scores[constraintCode]
	if !ok {
		w = &codeScoreWindow{}
		t.scores[constraintCode] = w
	}

	// Sliding window: keep last 2*minSamples scores
	maxWindow := t.minSamples * 2
	if maxWindow < 20 {
		maxWindow = 20
	}
	w.scores = append(w.scores, comprehension)
	if len(w.scores) > maxWindow {
		w.scores = w.scores[len(w.scores)-maxWindow:]
	}

	// Not enough samples yet
	if len(w.scores) < t.minSamples {
		return false
	}

	// Compute average over the window
	var sum float64
	for _, s := range w.scores {
		sum += s
	}
	avg := sum / float64(len(w.scores))

	if avg >= t.threshold {
		return false
	}

	// Check cooldown for this specific code
	if !w.lastRegen.IsZero() && time.Since(w.lastRegen) < t.cooldown {
		return false
	}

	// Check global rate limit
	now := time.Now()
	cutoff := now.Add(-time.Hour)
	recent := 0
	for _, ts := range t.regenLog {
		if ts.After(cutoff) {
			recent++
		}
	}
	if recent >= t.maxPerHour {
		return false
	}

	// Mark regen
	w.lastRegen = now
	w.scores = w.scores[:0] // reset window after regen trigger
	t.regenLog = append(t.regenLog, now)

	// Trim old entries from regenLog
	trimIdx := 0
	for i, ts := range t.regenLog {
		if ts.After(cutoff) {
			trimIdx = i
			break
		}
	}
	if trimIdx > 0 {
		t.regenLog = t.regenLog[trimIdx:]
	}

	return true
}

// TriggerRegen calls the code generator to regenerate a constraint code.
// This is a fire-and-forget async operation.
func (t *CodeComprehensionTracker) TriggerRegen(ctx context.Context, constraintCode, constraintType, description string) {
	if t.codeGen == nil {
		return
	}

	slog.Info("jiminy: code comprehension feedback loop triggered regen",
		"constraint_code", constraintCode,
		"constraint_type", constraintType,
	)

	newCode, err := t.codeGen.GenerateCode(ctx, constraintType, description)
	if err != nil {
		slog.Warn("jiminy: code regen failed", "constraint_code", constraintCode, "error", err)
		return
	}

	slog.Info("jiminy: code regen completed",
		"old_code", constraintCode,
		"new_code", newCode,
	)
}
