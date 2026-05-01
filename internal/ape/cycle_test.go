package ape

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/metrics"
)

// TestAcquireLLMSlot_NoSemaphore verifies that an orchestrator without an
// initialized semaphore (e.g. constructed by struct literal in some test
// fixtures) is a no-op that returns nil immediately.
func TestAcquireLLMSlot_NoSemaphore(t *testing.T) {
	c := &CycleOrchestrator{}
	if err := c.acquireLLMSlot(context.Background()); err != nil {
		t.Fatalf("acquireLLMSlot with nil semaphore returned %v, want nil", err)
	}
	c.releaseLLMSlot()
}

// TestAcquireLLMSlot_FastPath verifies the non-blocking send path: the first
// arrivals up to capacity acquire without incrementing the blocked counter.
func TestAcquireLLMSlot_FastPath(t *testing.T) {
	c := &CycleOrchestrator{llmSem: make(chan struct{}, 2)}
	before := metrics.Metrics().RSICLLMSemaphoreBlocked.Value()

	if err := c.acquireLLMSlot(context.Background()); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	if err := c.acquireLLMSlot(context.Background()); err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}

	after := metrics.Metrics().RSICLLMSemaphoreBlocked.Value()
	if after != before {
		t.Fatalf("blocked counter changed on fast path: before=%d after=%d", before, after)
	}
}

// TestAcquireLLMSlot_CtxCancelledWhileBlocked verifies that a goroutine
// waiting for a slot returns ctx.Err() if the context is cancelled before
// capacity frees up.
func TestAcquireLLMSlot_CtxCancelledWhileBlocked(t *testing.T) {
	c := &CycleOrchestrator{llmSem: make(chan struct{}, 1)}
	if err := c.acquireLLMSlot(context.Background()); err != nil {
		t.Fatalf("priming acquire failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := c.acquireLLMSlot(ctx)
	if err == nil {
		t.Fatal("expected acquire to fail with ctx.Err(), got nil")
	}
	c.releaseLLMSlot()
}

// TestAcquireLLMSlot_StressEightConcurrent reproduces the sprint-plan stress
// test: 8 goroutines try to acquire a 2-slot semaphore. Only 2 hold a slot at
// any moment, the blocked counter increments by 6 (the 6 contended goroutines
// each fail the non-blocking send before joining the waiting queue once), and
// each waiting goroutine eventually acquires + releases cleanly.
func TestAcquireLLMSlot_StressEightConcurrent(t *testing.T) {
	const capacity = 2
	const goroutines = 8
	c := &CycleOrchestrator{llmSem: make(chan struct{}, capacity)}

	before := metrics.Metrics().RSICLLMSemaphoreBlocked.Value()

	var inflight atomic.Int32
	var maxInflight atomic.Int32
	var completed atomic.Int32
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := c.acquireLLMSlot(ctx); err != nil {
				t.Errorf("acquire returned err: %v", err)
				return
			}
			n := inflight.Add(1)
			for {
				cur := maxInflight.Load()
				if n <= cur || maxInflight.CompareAndSwap(cur, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond) // hold the slot long enough that the others contend
			inflight.Add(-1)
			c.releaseLLMSlot()
			completed.Add(1)
		}()
	}
	close(start)
	wg.Wait()

	if got := completed.Load(); got != goroutines {
		t.Fatalf("completed=%d, want %d", got, goroutines)
	}
	if got := maxInflight.Load(); got > capacity {
		t.Fatalf("maxInflight=%d exceeded capacity=%d", got, capacity)
	}
	if got := inflight.Load(); got != 0 {
		t.Fatalf("inflight=%d, want 0 (semaphore not fully released)", got)
	}
	after := metrics.Metrics().RSICLLMSemaphoreBlocked.Value()
	delta := after - before
	wantMin := int64(goroutines - capacity)
	if delta < wantMin {
		t.Fatalf("blocked counter delta=%d, want >= %d", delta, wantMin)
	}
}

// TestNewCycleOrchestrator_LLMSemCapacity verifies that the constructor sizes
// the semaphore from cfg.RSICLLMConcurrencyLimit and clamps zero/negative to
// 1 (matching the FromEnv validator's guarantee but kept defensive in case a
// caller bypasses FromEnv).
func TestNewCycleOrchestrator_LLMSemCapacity(t *testing.T) {
	tests := []struct {
		name    string
		limit   int
		wantCap int
	}{
		{"explicit-2", 2, 2},
		{"explicit-4", 4, 4},
		{"zero-clamped-to-one", 0, 1},
		{"negative-clamped-to-one", -3, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{RSICLLMConcurrencyLimit: tt.limit}
			c := NewCycleOrchestrator(cfg, nil, nil, nil, nil, nil, nil, nil)
			if got := cap(c.llmSem); got != tt.wantCap {
				t.Fatalf("cap(llmSem)=%d, want %d", got, tt.wantCap)
			}
		})
	}
}
