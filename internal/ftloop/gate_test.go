package ftloop

import (
	"testing"
	"time"
)

// TestDecide_TruthTable pins the trigger-gate logic incl. the SF-2 suppressions.
func TestDecide_TruthTable(t *testing.T) {
	const interval = 168 * time.Hour
	cases := []struct {
		name                      string
		enabled, hasOpen, hasLast bool
		lastAge                   time.Duration
		fresh, minFresh           float64
		want                      Decision
	}{
		{"open cycle bars everything", true, true, true, 1000 * time.Hour, 1.0, 0.3, DecideSuppressOpenCycle},
		{"open bars even when disabled", false, true, false, 0, 1.0, 0.3, DecideSuppressOpenCycle},
		{"within interval suppresses", true, false, true, 10 * time.Hour, 1.0, 0.3, DecideSuppressInterval},
		{"disabled suppresses (SF-2)", false, false, true, 1000 * time.Hour, 1.0, 0.3, DecideSuppressDisabled},
		{"disabled, no prior cycle (SF-2)", false, false, false, 0, 1.0, 0.3, DecideSuppressDisabled},
		{"enabled but not fresh", true, false, true, 1000 * time.Hour, 0.1, 0.3, DecideSuppressNotFresh},
		{"enabled, fresh, clear -> trigger", true, false, true, 1000 * time.Hour, 0.5, 0.3, DecideTrigger},
		{"enabled, first ever cycle -> trigger (freshness skipped)", true, false, false, 0, 0.0, 0.3, DecideTrigger},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Decide(c.enabled, c.hasOpen, c.hasLast, c.lastAge, interval, c.fresh, c.minFresh)
			if got != c.want {
				t.Errorf("Decide = %q, want %q", got, c.want)
			}
			if c.want == DecideTrigger && got.IsSuppressed() {
				t.Error("trigger must not report suppressed")
			}
			if c.want != DecideTrigger && !got.IsSuppressed() {
				t.Error("non-trigger must report suppressed")
			}
		})
	}
}
