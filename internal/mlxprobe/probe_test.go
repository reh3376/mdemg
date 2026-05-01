package mlxprobe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// TestStateString verifies the stringer covers all states.
func TestStateString(t *testing.T) {
	cases := []struct {
		s    State
		want string
	}{
		{StateUp, "up"},
		{StateDegraded, "degraded"},
		{StateDown, "down"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("State(%d).String()=%q want %q", c.s, got, c.want)
		}
	}
}

// TestNewAppliesDefaults verifies zero-value Config gets sensible defaults.
func TestNewAppliesDefaults(t *testing.T) {
	p := New(Config{Endpoint: "http://x/v1"})
	if p.cfg.Interval != 5*time.Second {
		t.Errorf("default Interval=%v, want 5s", p.cfg.Interval)
	}
	if p.cfg.Timeout != 2*time.Second {
		t.Errorf("default Timeout=%v, want 2s", p.cfg.Timeout)
	}
	if p.cfg.FailureThreshold != 3 {
		t.Errorf("default FailureThreshold=%d, want 3", p.cfg.FailureThreshold)
	}
	if p.cfg.SuccessThreshold != 2 {
		t.Errorf("default SuccessThreshold=%d, want 2", p.cfg.SuccessThreshold)
	}
	if p.State() != StateUp {
		t.Errorf("initial State=%v, want StateUp", p.State())
	}
}

// TestRecordFailureTransitionsToDown verifies the failure-threshold ladder.
func TestRecordFailureTransitionsToDown(t *testing.T) {
	p := New(Config{Endpoint: "http://x/v1", FailureThreshold: 3})

	transitions := captureTransitions(p)

	p.recordFailure(errors.New("boom"))
	if p.State() != StateUp {
		t.Errorf("after 1 failure, State=%v, want StateUp", p.State())
	}
	p.recordFailure(errors.New("boom"))
	if p.State() != StateUp {
		t.Errorf("after 2 failures, State=%v, want StateUp", p.State())
	}
	p.recordFailure(errors.New("boom"))
	if p.State() != StateDown {
		t.Errorf("after 3 failures, State=%v, want StateDown", p.State())
	}

	if len(*transitions) != 1 {
		t.Errorf("expected 1 transition, got %d", len(*transitions))
	} else if (*transitions)[0].from != StateUp || (*transitions)[0].to != StateDown {
		t.Errorf("transition: from=%v to=%v, want up->down",
			(*transitions)[0].from, (*transitions)[0].to)
	}
}

// TestRecoveryRequiresTwoConsecutiveSuccesses verifies hysteresis: a single
// success after StateDown does NOT flip back to StateUp.
func TestRecoveryRequiresTwoConsecutiveSuccesses(t *testing.T) {
	p := New(Config{Endpoint: "http://x/v1", FailureThreshold: 1, SuccessThreshold: 2})

	p.recordFailure(errors.New("boom"))
	if p.State() != StateDown {
		t.Fatalf("expected StateDown after 1 failure, got %v", p.State())
	}

	p.recordSuccess(10 * time.Millisecond)
	if p.State() != StateDown {
		t.Errorf("after 1 success, State=%v, want StateDown (hysteresis)", p.State())
	}

	p.recordSuccess(10 * time.Millisecond)
	if p.State() != StateUp {
		t.Errorf("after 2 successes, State=%v, want StateUp", p.State())
	}
}

// TestSlowSuccessesTransitionToDegraded covers the up->degraded path.
func TestSlowSuccessesTransitionToDegraded(t *testing.T) {
	p := New(Config{
		Endpoint:              "http://x/v1",
		SlowResponseThreshold: 500 * time.Millisecond,
		SlowThreshold:         3,
	})

	p.recordSuccess(1 * time.Second) // slow
	p.recordSuccess(1 * time.Second) // slow
	if p.State() != StateUp {
		t.Errorf("after 2 slow successes, State=%v, want StateUp", p.State())
	}
	p.recordSuccess(1 * time.Second) // slow #3 -> degraded
	if p.State() != StateDegraded {
		t.Errorf("after 3 slow successes, State=%v, want StateDegraded", p.State())
	}

	// One normal success returns to up.
	p.recordSuccess(10 * time.Millisecond)
	if p.State() != StateUp {
		t.Errorf("after fast success from degraded, State=%v, want StateUp", p.State())
	}
}

// TestDegradedToDown verifies failures from StateDegraded still trip down.
func TestDegradedToDown(t *testing.T) {
	p := New(Config{
		Endpoint:              "http://x/v1",
		FailureThreshold:      2,
		SlowResponseThreshold: 500 * time.Millisecond,
		SlowThreshold:         1,
	})
	p.recordSuccess(1 * time.Second) // slow #1 -> degraded
	if p.State() != StateDegraded {
		t.Fatalf("expected StateDegraded, got %v", p.State())
	}
	p.recordFailure(errors.New("boom"))
	p.recordFailure(errors.New("boom"))
	if p.State() != StateDown {
		t.Errorf("after 2 failures from degraded, State=%v, want StateDown", p.State())
	}
}

// TestRunProbeAgainstFakeServer is a lightweight integration check that the
// Run loop polls a real (test) HTTP server and detects when it goes away.
func TestRunProbeAgainstFakeServer(t *testing.T) {
	healthy := atomic.Bool{}
	healthy.Store(true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !healthy.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	// Use very short interval/timeout for speed.
	p := New(Config{
		Endpoint:         srv.URL + "/v1",
		Interval:         50 * time.Millisecond,
		Timeout:          25 * time.Millisecond,
		FailureThreshold: 2,
		SuccessThreshold: 2,
	})

	transitions := captureTransitions(p)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = p.Run(ctx)
		close(done)
	}()

	// Allow a few successful probes.
	if !waitFor(func() bool { return p.State() == StateUp }, 1*time.Second) {
		cancel()
		<-done
		t.Fatalf("never observed StateUp; transitions=%v", transitions)
	}

	// Flip server to unhealthy. After FailureThreshold ticks the state
	// should go down.
	healthy.Store(false)
	if !waitFor(func() bool { return p.State() == StateDown }, 1500*time.Millisecond) {
		cancel()
		<-done
		t.Fatalf("never transitioned to StateDown; transitions=%v", transitions)
	}

	// Flip back to healthy. After SuccessThreshold ticks state should recover.
	healthy.Store(true)
	if !waitFor(func() bool { return p.State() == StateUp }, 1500*time.Millisecond) {
		cancel()
		<-done
		t.Fatalf("never recovered to StateUp; transitions=%v", transitions)
	}

	cancel()
	<-done
}

// TestRunCancelsCleanlyWithoutLeakingGoroutines verifies the goroutine count
// returns to baseline after Run returns.
func TestRunCancelsCleanlyWithoutLeakingGoroutines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	baseline := runtime.NumGoroutine()

	p := New(Config{
		Endpoint:         srv.URL + "/v1",
		Interval:         20 * time.Millisecond,
		Timeout:          10 * time.Millisecond,
		FailureThreshold: 1,
		SuccessThreshold: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = p.Run(ctx)
		close(done)
	}()

	// Let it run briefly.
	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Run did not return within 1s of ctx cancellation")
	}

	// Allow a tick for the http.Client to close idle conns.
	time.Sleep(50 * time.Millisecond)

	// Goroutine count should be at or below baseline + a small fudge for
	// http.Transport idle conns. A leak shows up as ≫ baseline.
	cur := runtime.NumGoroutine()
	if cur > baseline+5 {
		t.Errorf("goroutine leak: baseline=%d after=%d", baseline, cur)
	}
}

// TestEmptyEndpointReturnsError covers the misconfiguration guard.
func TestEmptyEndpointReturnsError(t *testing.T) {
	p := New(Config{})
	if err := p.Run(context.Background()); err == nil {
		t.Error("Run with empty Endpoint should return an error")
	}
}

// TestDefaultSingleton verifies SetDefault/Default are race-safe and reset
// cleanly.
func TestDefaultSingleton(t *testing.T) {
	if Default() != nil {
		t.Fatal("Default() must be nil before SetDefault")
	}

	p := New(Config{Endpoint: "http://x/v1"})
	SetDefault(p)
	if got := Default(); got != p {
		t.Errorf("Default()=%p, want %p", got, p)
	}

	SetDefault(nil)
	if Default() != nil {
		t.Error("Default() must be nil after SetDefault(nil)")
	}
}

// TestFastFailToggleAndObserver verifies the package-level toggle + observer
// wiring used by llmclient's gate.
func TestFastFailToggleAndObserver(t *testing.T) {
	t.Cleanup(func() {
		SetFastFailEnabled(false)
		SetFastFailObserver(nil)
	})

	if FastFailEnabled() {
		t.Fatal("FastFailEnabled() must default to false")
	}
	SetFastFailEnabled(true)
	if !FastFailEnabled() {
		t.Error("SetFastFailEnabled(true) did not flip the toggle")
	}

	type fired struct {
		task, base string
	}
	var got fired
	SetFastFailObserver(func(callerTask, baseURL string) {
		got = fired{task: callerTask, base: baseURL}
	})

	NotifyFastFail("consulting.classify", "http://127.0.0.1:8101/v1")
	if got.task != "consulting.classify" || got.base != "http://127.0.0.1:8101/v1" {
		t.Errorf("observer received task=%q base=%q, want consulting.classify + 8101", got.task, got.base)
	}

	// Clearing the observer must not panic.
	SetFastFailObserver(nil)
	NotifyFastFail("x", "y")
}

// --- helpers ----------------------------------------------------------------

type recordedTransition struct {
	from, to State
}

func captureTransitions(p *Prober) *[]recordedTransition {
	transitions := []recordedTransition{}
	mu := newMu()
	p.OnTransition(func(from, to State, _ error) {
		mu.Lock()
		defer mu.Unlock()
		transitions = append(transitions, recordedTransition{from: from, to: to})
	})
	return &transitions
}

// newMu returns a tiny RWMutex helper so the captureTransitions slice append
// is race-safe under -race when multiple goroutines could fire callbacks.
type miniMutex struct{ ch chan struct{} }

func newMu() *miniMutex { return &miniMutex{ch: make(chan struct{}, 1)} }

func (m *miniMutex) Lock()   { m.ch <- struct{}{} }
func (m *miniMutex) Unlock() { <-m.ch }

func waitFor(cond func() bool, max time.Duration) bool {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}
