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

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop after max retries")
	}

	// Should have attempted 4 times: 1 initial + 3 retries, then permanent failure
	if calls.Load() < 4 {
		t.Errorf("expected at least 4 calls, got %d", calls.Load())
	}

	alertMu.Lock()
	defer alertMu.Unlock()
	// Should have 3 restart alerts + 1 permanent failure alert
	if len(alerts) < 4 {
		t.Errorf("expected at least 4 alerts, got %d: %v", len(alerts), alerts)
	}

	// Last alert should be critical
	last := alerts[len(alerts)-1]
	if last[:8] != "critical" {
		t.Errorf("last alert should be critical, got %q", last)
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

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop after max retries")
	}

	// 1 initial + 2 retries = 3 calls
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
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

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("supervisor did not stop")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(times) < 3 {
		t.Fatalf("expected at least 3 timestamps, got %d", len(times))
	}

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
