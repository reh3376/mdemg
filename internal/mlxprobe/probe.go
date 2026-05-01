// Package mlxprobe runs a small background goroutine that polls an mlx_lm.server
// /v1/models endpoint to determine whether the local LLM backend is reachable.
//
// The probe maintains a 3-state health view per endpoint:
//   - StateUp:        recent probes succeeded
//   - StateDegraded:  recent probes were slow but still responding (informational only)
//   - StateDown:      recent probes failed or connection refused
//
// llmclient consults the package-level Default() singleton to fast-fail outbound
// LLM calls when the endpoint is StateDown, eliminating the retry-storm pattern
// that occurred when mlx died and 16 LLM call sites each independently retried
// six times with exponential backoff.
//
// State machine (with hysteresis to avoid flapping):
//   StateUp        -> StateDown:     N consecutive failures (default 3)
//   StateUp        -> StateDegraded: N consecutive slow successes (default 3)
//   StateDegraded  -> StateUp:       1 normal-latency success
//   StateDegraded  -> StateDown:     N consecutive failures
//   StateDown      -> StateUp:       M consecutive successes (default 2)
//
// The Prober integrates with internal/supervisor for panic recovery + ctx
// cancellation: register Run() as a supervised worker; the goroutine blocks
// until ctx is cancelled.
package mlxprobe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// State is the probe's health verdict for the endpoint.
type State int32

const (
	// StateUp means the most recent probe succeeded within the latency budget.
	StateUp State = 0
	// StateDegraded means probes are succeeding but exceeding the slow-response
	// threshold; informational only — does NOT trigger fast-fail.
	StateDegraded State = 1
	// StateDown means probes are failing; llmclient should fast-fail rather than
	// enter its retry loop.
	StateDown State = 2
)

// String returns a human-readable name for the state.
func (s State) String() string {
	switch s {
	case StateUp:
		return "up"
	case StateDegraded:
		return "degraded"
	case StateDown:
		return "down"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// Config tunes a Prober. Zero values fall back to sensible defaults; callers
// should populate from internal/config.Config knobs.
type Config struct {
	// Endpoint is the base URL of the mlx_lm.server (e.g. "http://127.0.0.1:8101/v1").
	// The probe issues GET <Endpoint>/models.
	Endpoint string

	// Interval is how often the probe runs. Default 5s.
	Interval time.Duration

	// Timeout is the per-probe HTTP timeout. Must be < Interval. Default 2s.
	Timeout time.Duration

	// FailureThreshold is the number of consecutive failures that triggers
	// StateUp/StateDegraded -> StateDown. Default 3.
	FailureThreshold int

	// SuccessThreshold is the number of consecutive successes that triggers
	// StateDown -> StateUp. Default 2.
	SuccessThreshold int

	// SlowResponseThreshold is the latency above which a successful probe is
	// considered "slow" and counts toward the degraded threshold. Default 1s.
	SlowResponseThreshold time.Duration

	// SlowThreshold is the number of consecutive slow successes that triggers
	// StateUp -> StateDegraded. Default 3.
	SlowThreshold int
}

// TransitionFunc is invoked whenever the probe's state changes. It is called
// from the probe's goroutine; callers should not block.
type TransitionFunc func(from, to State, lastErr error)

// Prober runs the periodic health check.
type Prober struct {
	cfg    Config
	client *http.Client

	// state is the current health verdict. Stored as atomic.Int32 (mirrors
	// circuitbreaker's atomic-state pattern).
	state atomic.Int32

	// lastError is the most recent error string (atomic.Value of *string for
	// race-free read across goroutines).
	lastError atomic.Pointer[probeError]

	// counters track consecutive observations.
	consecutiveFailures int
	consecutiveSlow     int
	consecutiveSuccess  int

	// callback fires on every state transition.
	mu       sync.Mutex
	callback TransitionFunc
}

// probeError wraps the most recent error with a timestamp so the watchdog CLI
// can show "down since X".
type probeError struct {
	Err  error
	When time.Time
}

// New returns a Prober with defaults applied.
func New(cfg Config) *Prober {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 2
	}
	if cfg.SlowResponseThreshold <= 0 {
		cfg.SlowResponseThreshold = 1 * time.Second
	}
	if cfg.SlowThreshold <= 0 {
		cfg.SlowThreshold = 3
	}

	p := &Prober{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
	p.state.Store(int32(StateUp))
	return p
}

// State returns the current health verdict. Safe to call from any goroutine.
func (p *Prober) State() State {
	return State(p.state.Load())
}

// Endpoint returns the endpoint this prober monitors.
func (p *Prober) Endpoint() string {
	return p.cfg.Endpoint
}

// LastError returns the most recent probe error (or nil if no error yet).
func (p *Prober) LastError() error {
	pe := p.lastError.Load()
	if pe == nil {
		return nil
	}
	return pe.Err
}

// LastErrorAt returns the timestamp of the most recent error.
func (p *Prober) LastErrorAt() time.Time {
	pe := p.lastError.Load()
	if pe == nil {
		return time.Time{}
	}
	return pe.When
}

// OnTransition registers a callback fired on every state change. Replaces any
// previous callback.
func (p *Prober) OnTransition(fn TransitionFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callback = fn
}

// Run drives the probe loop. It blocks until ctx is cancelled and returns nil.
// Designed to be registered with supervisor.Register().
func (p *Prober) Run(ctx context.Context) error {
	if p.cfg.Endpoint == "" {
		return errors.New("mlxprobe: empty Endpoint")
	}

	slog.Info("mlxprobe: starting", "endpoint", p.cfg.Endpoint,
		"interval", p.cfg.Interval, "timeout", p.cfg.Timeout)

	// Fire one immediate probe so the state isn't stuck at StateUp until the
	// first ticker tick.
	p.tick(ctx)

	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("mlxprobe: stopped", "endpoint", p.cfg.Endpoint)
			return nil
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

// tick runs a single probe and updates state.
func (p *Prober) tick(ctx context.Context) {
	start := time.Now()
	err := p.probeOnce(ctx)
	latency := time.Since(start)

	if err != nil {
		p.recordFailure(err)
		return
	}
	p.recordSuccess(latency)
}

// probeOnce issues a single GET <endpoint>/models and returns nil on 2xx.
func (p *Prober) probeOnce(ctx context.Context) error {
	probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, p.cfg.Endpoint+"/models", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func (p *Prober) recordFailure(err error) {
	p.lastError.Store(&probeError{Err: err, When: time.Now().UTC()})

	p.consecutiveFailures++
	p.consecutiveSlow = 0
	p.consecutiveSuccess = 0

	cur := p.State()
	if p.consecutiveFailures >= p.cfg.FailureThreshold && cur != StateDown {
		p.transitionTo(cur, StateDown, err)
	}
}

func (p *Prober) recordSuccess(latency time.Duration) {
	p.consecutiveFailures = 0

	if latency >= p.cfg.SlowResponseThreshold {
		p.consecutiveSlow++
		p.consecutiveSuccess = 0
	} else {
		p.consecutiveSlow = 0
		p.consecutiveSuccess++
	}

	cur := p.State()
	switch cur {
	case StateDown:
		// Need M consecutive successes (any latency) to recover.
		if p.consecutiveSuccess+p.consecutiveSlow >= p.cfg.SuccessThreshold {
			p.transitionTo(cur, StateUp, nil)
		}
	case StateUp:
		// Slow successes can degrade us.
		if p.consecutiveSlow >= p.cfg.SlowThreshold {
			p.transitionTo(cur, StateDegraded, nil)
		}
	case StateDegraded:
		// A single normal-latency success returns us to up.
		if p.consecutiveSuccess >= 1 {
			p.transitionTo(cur, StateUp, nil)
		}
	}
}

// transitionTo atomically swaps state and fires the callback.
func (p *Prober) transitionTo(from, to State, lastErr error) {
	if !p.state.CompareAndSwap(int32(from), int32(to)) {
		// Concurrent transition raced us; respect their result.
		return
	}
	slog.Info("mlxprobe: state transition",
		"endpoint", p.cfg.Endpoint, "from", from, "to", to, "error", lastErr)

	p.mu.Lock()
	cb := p.callback
	p.mu.Unlock()
	if cb != nil {
		cb(from, to, lastErr)
	}
}

// --- Package-level singleton wiring -----------------------------------------
//
// llmclient consults the singleton via Default() to decide whether to short-
// circuit doWithRetry. The pattern mirrors llmclient.SetDefaultRecorder /
// SetDefaultAlertCallback used elsewhere in the codebase.

var (
	defaultMu     sync.RWMutex
	defaultProber *Prober

	// fastFailEnabled gates the llmclient short-circuit. Independent of
	// SetDefault: the operator may want to observe state (metrics, alerts)
	// without altering retry behaviour during rollout.
	fastFailEnabled atomic.Bool

	// fastFailObserverMu guards fastFailObserver. The observer is fired
	// whenever llmclient short-circuits a call; the wiring layer uses it to
	// increment the mdemg_mlx_fast_fail_total counter without forcing
	// llmclient to import the metrics package.
	fastFailObserverMu sync.RWMutex
	fastFailObserver   func(callerTask, baseURL string)
)

// SetDefault registers p as the package-wide Prober. Pass nil to clear.
// Callers must invoke this BEFORE any llmclient is constructed; llmclient
// reads the singleton at request time, so a later SetDefault() is honoured by
// future calls but in-flight retry loops will not pick it up.
func SetDefault(p *Prober) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultProber = p
}

// Default returns the package-wide Prober, or nil if SetDefault was never
// called (e.g. in unit tests, or when MLX_WATCHDOG_ENABLED=false).
func Default() *Prober {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultProber
}

// SetFastFailEnabled toggles the llmclient short-circuit behaviour. Default
// is false; the operator opts in via MLX_FAIL_FAST_ENABLED.
func SetFastFailEnabled(b bool) {
	fastFailEnabled.Store(b)
}

// FastFailEnabled returns the current fast-fail toggle.
func FastFailEnabled() bool {
	return fastFailEnabled.Load()
}

// SetFastFailObserver registers a callback invoked whenever llmclient
// short-circuits a call. Pass nil to clear.
func SetFastFailObserver(fn func(callerTask, baseURL string)) {
	fastFailObserverMu.Lock()
	defer fastFailObserverMu.Unlock()
	fastFailObserver = fn
}

// NotifyFastFail invokes the registered observer (if any). llmclient calls
// this from inside doWithRetry when it short-circuits.
func NotifyFastFail(callerTask, baseURL string) {
	fastFailObserverMu.RLock()
	fn := fastFailObserver
	fastFailObserverMu.RUnlock()
	if fn != nil {
		fn(callerTask, baseURL)
	}
}

// ForceStateForTest is a TEST-ONLY helper that bypasses the state machine and
// drops the prober directly into the requested state. Production code MUST
// NOT call this — use the natural transition path via Run().
//
// This exists because llmclient's fast-fail gate keys on Prober.Endpoint() ==
// Client.baseURL, which means tests need a Prober that simultaneously claims
// a canonical endpoint AND is in StateDown without requiring an actual server
// listening at that endpoint.
func ForceStateForTest(p *Prober, s State) {
	p.state.Store(int32(s))
}
