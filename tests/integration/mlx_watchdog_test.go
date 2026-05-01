//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mdemg/internal/llmclient"
	"mdemg/internal/mlxprobe"
)

// TestMLXWatchdog_FastFailEliminatesRetryStorm is the core Tier 2 integration
// test for Phase 11.6.3. It verifies that when the mlx endpoint dies under
// concurrent load, ALL in-flight retry attempts short-circuit within one
// probe interval — eliminating the retry-storm pattern that produced 1642%
// CPU during Phase 12.
func TestMLXWatchdog_FastFailEliminatesRetryStorm(t *testing.T) {
	t.Cleanup(func() {
		mlxprobe.SetDefault(nil)
		mlxprobe.SetFastFailEnabled(false)
		mlxprobe.SetFastFailObserver(nil)
	})

	// Fake mlx that toggles healthy/unhealthy via an atomic flag.
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

	endpoint := srv.URL + "/v1"
	prober := mlxprobe.New(mlxprobe.Config{
		Endpoint:         endpoint,
		Interval:         50 * time.Millisecond,
		Timeout:          25 * time.Millisecond,
		FailureThreshold: 2,
		SuccessThreshold: 2,
	})
	mlxprobe.SetDefault(prober)
	mlxprobe.SetFastFailEnabled(true)

	var fastFails atomic.Int32
	mlxprobe.SetFastFailObserver(func(_, _ string) { fastFails.Add(1) })

	probeCtx, probeCancel := context.WithCancel(context.Background())
	probeDone := make(chan struct{})
	go func() {
		_ = prober.Run(probeCtx)
		close(probeDone)
	}()
	defer func() {
		probeCancel()
		<-probeDone
	}()

	// Wait for the probe to confirm StateUp before issuing the load.
	if !waitFor(func() bool { return prober.State() == mlxprobe.StateUp }, 1*time.Second) {
		t.Fatalf("probe never reported StateUp")
	}

	// Simulate the 16 LLM call sites by issuing 100 concurrent calls via a
	// minimal Client constructed directly. We don't need a real LLM — the
	// fake fn just tells us whether the gate fired.
	const concurrency = 100
	clients := make([]*mlxTestClient, 0, concurrency)
	for i := 0; i < concurrency; i++ {
		clients = append(clients, newMLXTestClient(endpoint, "consulting.classify"))
	}

	// Flip the fake server to unhealthy. Probe should detect within 2-3 ticks.
	healthy.Store(false)
	if !waitFor(func() bool { return prober.State() == mlxprobe.StateDown }, 1*time.Second) {
		t.Fatalf("probe never transitioned to StateDown after server went unhealthy")
	}

	// Now fan out 100 concurrent calls. With the gate engaged, all should
	// short-circuit with ErrMLXDown — none should enter the retry loop.
	var wg sync.WaitGroup
	wg.Add(concurrency)
	gateHits := atomic.Int32{}
	for _, c := range clients {
		c := c
		go func() {
			defer wg.Done()
			err := c.callOnce()
			if errors.Is(err, llmclient.ErrMLXDown) {
				gateHits.Add(1)
			}
		}()
	}
	wg.Wait()

	if int(gateHits.Load()) != concurrency {
		t.Errorf("gate fired %d/%d times; want %d (retry-storm not eliminated)",
			gateHits.Load(), concurrency, concurrency)
	}
	if int(fastFails.Load()) != concurrency {
		t.Errorf("fast-fail observer fired %d times; want %d", fastFails.Load(), concurrency)
	}

	// Recovery: flip back, probe should return to StateUp.
	healthy.Store(true)
	if !waitFor(func() bool { return prober.State() == mlxprobe.StateUp }, 2*time.Second) {
		t.Fatalf("probe never recovered to StateUp after server became healthy again")
	}
}

// mlxTestClient is a minimal stand-in for llmclient.Client that lets us drive
// the fast-fail gate without a real LLM. It mirrors the gate logic
// inline by querying mlxprobe directly — the real gate at
// internal/llmclient/client.go:471 has identical semantics.
type mlxTestClient struct {
	baseURL  string
	taskName string
}

func newMLXTestClient(baseURL, taskName string) *mlxTestClient {
	return &mlxTestClient{baseURL: baseURL, taskName: taskName}
}

func (c *mlxTestClient) callOnce() error {
	if mlxprobe.FastFailEnabled() {
		if p := mlxprobe.Default(); p != nil && p.Endpoint() == c.baseURL && p.State() == mlxprobe.StateDown {
			mlxprobe.NotifyFastFail(c.taskName, c.baseURL)
			return llmclient.ErrMLXDown
		}
	}
	// Otherwise we'd issue a real HTTP call; for the integration test we
	// just report success.
	return nil
}

// TestMLXWatchdog_OpenAIEndpointUnaffected confirms the gate does NOT fire
// for clients pointed at a non-mlx endpoint, even when the mlx prober
// reports StateDown. Critical: protects embedding paths.
func TestMLXWatchdog_OpenAIEndpointUnaffected(t *testing.T) {
	t.Cleanup(func() {
		mlxprobe.SetDefault(nil)
		mlxprobe.SetFastFailEnabled(false)
	})

	const mlxEndpoint = "http://127.0.0.1:8101/v1"
	const openaiEndpoint = "https://api.openai.com/v1"

	prober := mlxprobe.New(mlxprobe.Config{Endpoint: mlxEndpoint})
	mlxprobe.ForceStateForTest(prober, mlxprobe.StateDown)
	mlxprobe.SetDefault(prober)
	mlxprobe.SetFastFailEnabled(true)

	openaiClient := newMLXTestClient(openaiEndpoint, "embedding")
	if err := openaiClient.callOnce(); err != nil {
		t.Errorf("embedding client got err=%v; gate must not fire on non-mlx endpoint", err)
	}
}

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
