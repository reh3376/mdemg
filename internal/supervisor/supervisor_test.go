package supervisor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSupervisor_NormalShutdown(t *testing.T) {
	var started atomic.Int32
	s := New(nil)
	s.Register("test-worker", func(ctx context.Context) error {
		started.Add(1)
		<-ctx.Done()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	// Give worker time to start
	time.Sleep(50 * time.Millisecond)
	if started.Load() != 1 {
		t.Fatal("worker should have started")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop in time")
	}
}

func TestSupervisor_PanicRecovery(t *testing.T) {
	var calls atomic.Int32
	var alertMu sync.Mutex
	var alerts []string

	s := New(func(service, title, message, severity string) {
		alertMu.Lock()
		alerts = append(alerts, fmt.Sprintf("%s:%s", severity, message))
		alertMu.Unlock()
	})
	s.backoff = 10 * time.Millisecond // fast for testing

	s.Register("panic-worker", func(_ context.Context) error {
		n := calls.Add(1)
		panic(fmt.Sprintf("panic #%d", n))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	// The supervisor stays alive after a worker fails permanently (it may
	// host late workers), so poll for the terminal critical alert.
	deadline := time.Now().Add(5 * time.Second)
	for {
		alertMu.Lock()
		n := len(alerts)
		var last string
		if n > 0 {
			last = alerts[n-1]
		}
		alertMu.Unlock()
		if n >= 4 && last[:8] == "critical" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker did not fail permanently; alerts: %v", alerts)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Should have attempted 4 times: 1 initial + 3 retries, then permanent failure
	if calls.Load() < 4 {
		t.Errorf("expected at least 4 calls, got %d", calls.Load())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop after cancel")
	}
}

func TestSupervisor_ErrorRestart(t *testing.T) {
	var calls atomic.Int32

	s := New(nil)
	s.backoff = 10 * time.Millisecond
	s.maxRetry = 2

	s.Register("error-worker", func(ctx context.Context) error {
		calls.Add(1)
		return fmt.Errorf("simulated error")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	// 1 initial + 2 retries = 3 calls, then permanent failure (supervisor stays up)
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() != 3 {
		if time.Now().After(deadline) {
			t.Fatalf("expected 3 calls, got %d", calls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Settle: ensure no 4th call arrives (budget exhausted within window)
	time.Sleep(100 * time.Millisecond)
	if calls.Load() != 3 {
		t.Errorf("expected exactly 3 calls, got %d", calls.Load())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop after cancel")
	}
}

func TestSupervisor_BackoffIncreases(t *testing.T) {
	var times []time.Time
	var mu sync.Mutex

	s := New(nil)
	s.backoff = 50 * time.Millisecond
	s.maxRetry = 3

	s.Register("backoff-worker", func(ctx context.Context) error {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		return fmt.Errorf("fail")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		mu.Lock()
		n := len(times)
		mu.Unlock()
		if n >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected at least 3 timestamps, got %d", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop after cancel")
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify delays increase: delay2 > delay1
	delay1 := times[1].Sub(times[0])
	delay2 := times[2].Sub(times[1])
	if delay2 <= delay1 {
		t.Errorf("expected increasing backoff: delay1=%v, delay2=%v", delay1, delay2)
	}
}

func TestSupervisor_MultipleWorkers(t *testing.T) {
	var started atomic.Int32

	s := New(nil)
	for i := range 3 {
		name := fmt.Sprintf("worker-%d", i)
		s.Register(name, func(ctx context.Context) error {
			started.Add(1)
			<-ctx.Done()
			return nil
		})
	}

	if s.WorkerCount() != 3 {
		t.Fatalf("expected 3 workers, got %d", s.WorkerCount())
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	if started.Load() != 3 {
		t.Errorf("expected 3 started workers, got %d", started.Load())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop in time")
	}
}

func TestSupervisor_StopMethod(t *testing.T) {
	s := New(nil)
	s.Register("block-worker", func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	s.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not terminate supervisor")
	}
}

func TestSupervisor_WindowReplenishes(t *testing.T) {
	// With a fake clock that jumps far past the window on every read, each
	// restart is the only one "in window" — the budget replenishes and the
	// worker is never killed permanently, despite failing more than maxRetry
	// times in total.
	var calls atomic.Int32
	var criticals atomic.Int32

	s := New(func(_, _, _, severity string) {
		if severity == "critical" {
			criticals.Add(1)
		}
	})
	s.backoff = time.Millisecond
	s.maxRetry = 2
	s.window = time.Second

	var fake atomic.Int64
	s.now = func() time.Time {
		// each call advances 10× the window
		return time.Unix(0, fake.Add(int64(10*time.Second)))
	}

	s.Register("transient-worker", func(ctx context.Context) error {
		if calls.Add(1) <= 6 {
			return fmt.Errorf("transient #%d", calls.Load())
		}
		<-ctx.Done() // healthy from call 7 on
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 7 {
		if time.Now().After(deadline) {
			t.Fatalf("worker was killed before budget replenished: %d calls, %d criticals",
				calls.Load(), criticals.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}

	if criticals.Load() != 0 {
		t.Errorf("expected no permanent failures, got %d critical alerts", criticals.Load())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestSupervisor_PermanentFailureWithinWindow(t *testing.T) {
	// Real clock: rapid failures land inside one window and exhaust the budget.
	var calls atomic.Int32
	var criticals atomic.Int32

	s := New(func(_, _, _, severity string) {
		if severity == "critical" {
			criticals.Add(1)
		}
	})
	s.backoff = time.Millisecond
	s.maxRetry = 2
	s.window = time.Hour

	s.Register("crash-loop", func(_ context.Context) error {
		calls.Add(1)
		return fmt.Errorf("always fails")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for criticals.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("crash-looping worker was not failed permanently")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() != 3 { // 1 initial + maxRetry restarts
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}

	cancel()
	<-done
}

func TestSupervisor_GoAfterStart(t *testing.T) {
	var lateStarted atomic.Int32

	s := New(nil)
	s.Register("base-worker", func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)

	s.Go("late-worker", func(ctx context.Context) error {
		lateStarted.Add(1)
		<-ctx.Done()
		return nil
	})

	deadline := time.Now().Add(2 * time.Second)
	for lateStarted.Load() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("late worker did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if s.WorkerCount() != 2 {
		t.Errorf("expected 2 workers, got %d", s.WorkerCount())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not drain late worker on stop")
	}
}

func TestSupervisor_NilReturnNotRestarted(t *testing.T) {
	var calls atomic.Int32
	var alerted atomic.Int32

	s := New(func(_, _, _, _ string) { alerted.Add(1) })
	s.backoff = time.Millisecond

	s.Register("oneshot", func(_ context.Context) error {
		calls.Add(1)
		return nil // intentional completion (e.g. own stop channel)
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond)
	if calls.Load() != 1 {
		t.Errorf("nil-returning worker should run exactly once, got %d", calls.Load())
	}
	if alerted.Load() != 0 {
		t.Errorf("nil return should not alert, got %d alerts", alerted.Load())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestSupervisor_Configure(t *testing.T) {
	s := New(nil)
	s.Configure(5, 10*time.Minute, 2*time.Second)
	if s.maxRetry != 5 || s.window != 10*time.Minute || s.backoff != 2*time.Second {
		t.Errorf("Configure did not apply: %d %v %v", s.maxRetry, s.window, s.backoff)
	}
	// Zero/negative keep current values
	s.Configure(0, -time.Minute, 0)
	if s.maxRetry != 5 || s.window != 10*time.Minute || s.backoff != 2*time.Second {
		t.Errorf("Configure overwrote with invalid values: %d %v %v", s.maxRetry, s.window, s.backoff)
	}
}
