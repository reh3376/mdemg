package llmclient

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"mdemg/internal/mlxprobe"
)

// TestDoWithRetry_FastFailsWhenDown verifies the gate short-circuits the retry
// loop when the prober reports StateDown AND fast-fail is enabled.
func TestDoWithRetry_FastFailsWhenDown(t *testing.T) {
	t.Cleanup(func() {
		mlxprobe.SetDefault(nil)
		mlxprobe.SetFastFailEnabled(false)
		mlxprobe.SetFastFailObserver(nil)
	})

	const endpoint = "http://127.0.0.1:8101/v1"
	p := mlxprobe.New(mlxprobe.Config{Endpoint: endpoint})
	mlxprobe.ForceStateForTest(p, mlxprobe.StateDown)
	mlxprobe.SetDefault(p)
	mlxprobe.SetFastFailEnabled(true)

	var observed atomic.Int32
	mlxprobe.SetFastFailObserver(func(_, _ string) {
		observed.Add(1)
	})

	c := &Client{
		baseURL:  endpoint,
		taskName: "consulting.classify",
		retryCfg: RetryConfig{Enabled: true, MaxAttempts: 5, BaseDelayMs: 1, MaxDelayMs: 1, Multiplier: 1, Jitter: 0},
	}

	called := atomic.Int32{}
	_, _, err := c.doWithRetry(context.Background(), func() (string, int, error) {
		called.Add(1)
		return "", 0, errors.New("fn should not be called")
	})

	if !errors.Is(err, ErrMLXDown) {
		t.Errorf("doWithRetry err=%v, want ErrMLXDown", err)
	}
	if called.Load() != 0 {
		t.Errorf("fn was called %d times — gate should short-circuit before fn", called.Load())
	}
	if observed.Load() != 1 {
		t.Errorf("observer fired %d times, want 1", observed.Load())
	}
}

// TestDoWithRetry_PassesThroughWhenUp verifies a healthy prober does not block
// normal calls.
func TestDoWithRetry_PassesThroughWhenUp(t *testing.T) {
	t.Cleanup(func() {
		mlxprobe.SetDefault(nil)
		mlxprobe.SetFastFailEnabled(false)
	})

	p := mlxprobe.New(mlxprobe.Config{Endpoint: "http://127.0.0.1:8101/v1"})
	mlxprobe.SetDefault(p)
	mlxprobe.SetFastFailEnabled(true)

	c := &Client{
		baseURL:  "http://127.0.0.1:8101/v1",
		taskName: "consulting.classify",
		retryCfg: RetryConfig{Enabled: false, MaxAttempts: 0},
	}

	got, _, err := c.doWithRetry(context.Background(), func() (string, int, error) {
		return "ok", 0, nil
	})
	if err != nil {
		t.Errorf("err=%v, want nil", err)
	}
	if got != "ok" {
		t.Errorf("got=%q, want \"ok\"", got)
	}
}

// TestDoWithRetry_GateSkippedForOpenAIBaseURL verifies the gate does NOT fire
// for clients pointed at the OpenAI endpoint, even when the mlx prober is
// down. This protects embedding paths.
func TestDoWithRetry_GateSkippedForOpenAIBaseURL(t *testing.T) {
	t.Cleanup(func() {
		mlxprobe.SetDefault(nil)
		mlxprobe.SetFastFailEnabled(false)
	})

	p := mlxprobe.New(mlxprobe.Config{Endpoint: "http://127.0.0.1:8101/v1"})
	mlxprobe.ForceStateForTest(p, mlxprobe.StateDown)
	mlxprobe.SetDefault(p)
	mlxprobe.SetFastFailEnabled(true)

	c := &Client{
		baseURL:  "https://api.openai.com/v1", // different endpoint
		taskName: "embedding",
		retryCfg: RetryConfig{Enabled: false, MaxAttempts: 0},
	}

	got, _, err := c.doWithRetry(context.Background(), func() (string, int, error) {
		return "ok", 0, nil
	})
	if err != nil {
		t.Errorf("err=%v, want nil — embedding endpoint must not be gated", err)
	}
	if got != "ok" {
		t.Errorf("got=%q, want \"ok\"", got)
	}
}

// TestDoWithRetry_GateRespectsFastFailToggle verifies that even with a
// downed prober, calls flow through normally when MLX_FAIL_FAST_ENABLED is
// false. (Operator escape hatch.)
func TestDoWithRetry_GateRespectsFastFailToggle(t *testing.T) {
	t.Cleanup(func() {
		mlxprobe.SetDefault(nil)
		mlxprobe.SetFastFailEnabled(false)
	})

	p := mlxprobe.New(mlxprobe.Config{Endpoint: "http://127.0.0.1:8101/v1"})
	mlxprobe.ForceStateForTest(p, mlxprobe.StateDown)
	mlxprobe.SetDefault(p)
	mlxprobe.SetFastFailEnabled(false) // toggle off

	c := &Client{
		baseURL:  "http://127.0.0.1:8101/v1",
		taskName: "consulting.classify",
		retryCfg: RetryConfig{Enabled: false, MaxAttempts: 0},
	}

	got, _, err := c.doWithRetry(context.Background(), func() (string, int, error) {
		return "ok", 0, nil
	})
	if err != nil {
		t.Errorf("err=%v, want nil — toggle off must skip gate", err)
	}
	if got != "ok" {
		t.Errorf("got=%q, want \"ok\"", got)
	}
}

// TestDoWithRetry_NoProberMeansFailOpen verifies that when SetDefault was
// never called (e.g. MLX_WATCHDOG_ENABLED=false), the gate is a no-op.
func TestDoWithRetry_NoProberMeansFailOpen(t *testing.T) {
	t.Cleanup(func() {
		mlxprobe.SetFastFailEnabled(false)
	})

	mlxprobe.SetDefault(nil)
	mlxprobe.SetFastFailEnabled(true) // toggle on but no prober → no-op

	c := &Client{
		baseURL:  "http://127.0.0.1:8101/v1",
		taskName: "consulting.classify",
		retryCfg: RetryConfig{Enabled: false, MaxAttempts: 0},
	}

	got, _, err := c.doWithRetry(context.Background(), func() (string, int, error) {
		return "ok", 0, nil
	})
	if err != nil {
		t.Errorf("err=%v, want nil — no prober must fail-open", err)
	}
	if got != "ok" {
		t.Errorf("got=%q, want \"ok\"", got)
	}
}
