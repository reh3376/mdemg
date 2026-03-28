package healthprobe

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProber_APIProbe_Healthy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p := New(time.Hour, ts.URL, nil, nil) // long interval, we'll call manually
	p.results = make(map[string]ProbeResult)
	p.runProbes()

	r := p.Results()

	api, ok := r["mdemg_api"]
	if !ok {
		t.Fatal("missing mdemg_api probe result")
	}
	if !api.Healthy {
		t.Errorf("expected API healthy, got error: %s", api.Error)
	}
	if api.Latency <= 0 {
		t.Error("expected positive latency")
	}
}

func TestProber_APIProbe_Unhealthy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := New(time.Hour, ts.URL, nil, nil)
	p.results = make(map[string]ProbeResult)
	p.runProbes()

	r := p.Results()

	api := r["mdemg_api"]
	if api.Healthy {
		t.Error("expected API unhealthy for 500 response")
	}
	if api.Error == "" {
		t.Error("expected error message for 500 response")
	}
}

func TestProber_NilDependencies(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p := New(time.Hour, ts.URL, nil, nil)
	p.results = make(map[string]ProbeResult)
	p.runProbes()

	r := p.Results()

	// Neo4j and TSDB should have "not configured" errors
	neo4j := r["neo4j"]
	if neo4j.Healthy {
		t.Error("expected neo4j unhealthy when nil driver")
	}
	if neo4j.Error != "not configured" {
		t.Errorf("expected 'not configured' error, got %q", neo4j.Error)
	}

	tsdb := r["timescaledb"]
	if tsdb.Healthy {
		t.Error("expected tsdb unhealthy when nil client")
	}
	if tsdb.Error != "not configured" {
		t.Errorf("expected 'not configured' error, got %q", tsdb.Error)
	}
}

func TestProber_StartStop(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	p := New(50*time.Millisecond, ts.URL, nil, nil)
	p.Start()

	// Wait for a couple of ticks
	time.Sleep(150 * time.Millisecond)

	// Stop should not hang
	done := make(chan struct{})
	go func() {
		p.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() hung for more than 2 seconds")
	}

	// Should have results from the probes
	r := p.Results()
	if len(r) == 0 {
		t.Error("expected probe results after running")
	}
}

func TestProber_SidecarProbe_Healthy(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer sidecar.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	p := New(time.Hour, api.URL, nil, nil, sidecar.URL)
	p.results = make(map[string]ProbeResult)
	p.runProbes()

	r := p.Results()
	sc, ok := r["neural_sidecar"]
	if !ok {
		t.Fatal("missing neural_sidecar probe result")
	}
	if !sc.Healthy {
		t.Errorf("expected sidecar healthy, got error: %s", sc.Error)
	}
	if sc.Latency <= 0 {
		t.Error("expected positive latency")
	}
}

func TestProber_SidecarProbe_Unhealthy(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sidecar.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	p := New(time.Hour, api.URL, nil, nil, sidecar.URL)
	p.results = make(map[string]ProbeResult)
	p.runProbes()

	r := p.Results()
	sc := r["neural_sidecar"]
	if sc.Healthy {
		t.Error("expected sidecar unhealthy for 500 response")
	}
	if sc.Error == "" {
		t.Error("expected error message for 500 response")
	}
}

func TestProber_SidecarProbe_NotConfigured(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	p := New(time.Hour, api.URL, nil, nil) // no sidecar URL
	p.results = make(map[string]ProbeResult)
	p.runProbes()

	r := p.Results()
	sc := r["neural_sidecar"]
	if sc.Healthy {
		t.Error("expected sidecar unhealthy when not configured")
	}
	if sc.Error != "not configured" {
		t.Errorf("expected 'not configured' error, got %q", sc.Error)
	}
}

func TestProber_StopWithoutStart(t *testing.T) {
	p := New(time.Hour, "http://localhost:9999", nil, nil)
	// Should not panic
	p.Stop()
}

func TestProber_Results_IsCopy(t *testing.T) {
	p := New(time.Hour, "http://localhost:9999", nil, nil)
	p.results = make(map[string]ProbeResult)
	p.results["test"] = ProbeResult{Target: "test", Healthy: true}

	r := p.Results()
	r["test"] = ProbeResult{Target: "test", Healthy: false}

	// Original should not be affected
	if !p.results["test"].Healthy {
		t.Error("modifying Results() copy should not affect original")
	}
}
