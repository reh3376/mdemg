// Package healthprobe provides a lightweight background prober that checks
// the health of MDEMG dependencies (API, Neo4j, TimescaleDB) and records
// probe results as metrics. This replaces the Prometheus blackbox-exporter
// container with native Go probes.
package healthprobe

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/metrics"
	"mdemg/internal/tsdb"
)

// ProbeResult represents the result of a single probe.
type ProbeResult struct {
	Target  string        `json:"target"`
	Healthy bool          `json:"healthy"`
	Latency time.Duration `json:"latency_ms"`
	Error   string        `json:"error,omitempty"`
	CheckAt time.Time     `json:"checked_at"`
}

// Prober runs periodic health probes and records results as metrics.
type Prober struct {
	interval   time.Duration
	apiURL     string
	sidecarURL string
	neo4j      neo4j.DriverWithContext
	tsdb       *tsdb.Client

	mu      sync.RWMutex
	results map[string]ProbeResult

	stopCh chan struct{}
	doneCh chan struct{}
}

// New creates a Prober.
//   - apiURL: the MDEMG API base URL (e.g., "http://localhost:9999")
//   - neo4jDriver: may be nil if Neo4j is not configured
//   - tsdbClient: may be nil if TSDB is not configured
//   - sidecarURL: the neural sidecar base URL (e.g., "http://localhost:8100"); empty to skip
func New(interval time.Duration, apiURL string, neo4jDriver neo4j.DriverWithContext, tsdbClient *tsdb.Client, sidecarURL ...string) *Prober {
	p := &Prober{
		interval: interval,
		apiURL:   apiURL,
		neo4j:    neo4jDriver,
		tsdb:     tsdbClient,
		results:  make(map[string]ProbeResult),
	}
	if len(sidecarURL) > 0 {
		p.sidecarURL = sidecarURL[0]
	}
	return p
}

// Start begins background probing. Call Stop to halt.
func (p *Prober) Start() {
	p.stopCh = make(chan struct{})
	p.doneCh = make(chan struct{})

	go func() {
		defer close(p.doneCh)
		// Run immediately on startup
		p.runProbes()

		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				p.runProbes()
			case <-p.stopCh:
				return
			}
		}
	}()
}

// Stop halts background probing.
func (p *Prober) Stop() {
	if p.stopCh == nil {
		return
	}
	close(p.stopCh)
	<-p.doneCh
}

// Results returns a snapshot of the latest probe results.
func (p *Prober) Results() map[string]ProbeResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make(map[string]ProbeResult, len(p.results))
	for k, v := range p.results {
		out[k] = v
	}
	return out
}

func (p *Prober) runProbes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p.probeAPI(ctx)
	p.probeNeo4j(ctx)
	p.probeTSDB(ctx)
	p.probeSidecar(ctx)

	p.recordMetrics()
}

func (p *Prober) probeAPI(ctx context.Context) {
	start := time.Now()
	result := ProbeResult{
		Target:  "mdemg_api",
		CheckAt: start,
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.apiURL+"/healthz", nil)
	if err != nil {
		result.Error = err.Error()
		p.store(result)
		return
	}

	resp, err := client.Do(req)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err.Error()
	} else {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			result.Healthy = true
		} else {
			result.Error = fmt.Sprintf("status %d", resp.StatusCode)
		}
	}
	p.store(result)
}

func (p *Prober) probeNeo4j(ctx context.Context) {
	start := time.Now()
	result := ProbeResult{
		Target:  "neo4j",
		CheckAt: start,
	}

	if p.neo4j == nil {
		result.Error = "not configured"
		p.store(result)
		return
	}

	err := p.neo4j.VerifyConnectivity(ctx)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err.Error()
	} else {
		result.Healthy = true
	}
	p.store(result)
}

func (p *Prober) probeTSDB(ctx context.Context) {
	start := time.Now()
	result := ProbeResult{
		Target:  "timescaledb",
		CheckAt: start,
	}

	if p.tsdb == nil {
		result.Error = "not configured"
		p.store(result)
		return
	}

	err := p.tsdb.Ping(ctx)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err.Error()
	} else {
		result.Healthy = true
	}
	p.store(result)
}

func (p *Prober) probeSidecar(ctx context.Context) {
	start := time.Now()
	result := ProbeResult{
		Target:  "neural_sidecar",
		CheckAt: start,
	}

	if p.sidecarURL == "" {
		result.Error = "not configured"
		p.store(result)
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.sidecarURL+"/health", nil)
	if err != nil {
		result.Error = err.Error()
		p.store(result)
		return
	}

	resp, err := client.Do(req)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err.Error()
	} else {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			result.Healthy = true
		} else {
			result.Error = fmt.Sprintf("status %d", resp.StatusCode)
		}
	}
	p.store(result)
}

func (p *Prober) store(r ProbeResult) {
	p.mu.Lock()
	p.results[r.Target] = r
	p.mu.Unlock()
}

func (p *Prober) recordMetrics() {
	reg := metrics.Metrics().Registry()
	p.mu.RLock()
	defer p.mu.RUnlock()

	for target, r := range p.results {
		var val float64
		if r.Healthy {
			val = 1
		}
		labels := map[string]string{"target": target}
		reg.NewGauge("probe_up", "Health probe status (1=up, 0=down)", labels).Set(val)
		reg.NewGauge("probe_latency_ms", "Health probe latency in ms", labels).Set(float64(r.Latency.Milliseconds()))
	}

	slog.Debug("health_probes: completed",
		"targets", len(p.results),
		"api", p.results["mdemg_api"].Healthy,
		"neo4j", p.results["neo4j"].Healthy,
		"tsdb", p.results["timescaledb"].Healthy,
	)
}
