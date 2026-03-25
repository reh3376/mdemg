package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// EventHandler implements Phase 1 background polling for change detection.
type EventHandler struct {
	specIndex    *SpecIndex
	resultClient *ResultClient
	config       pluginConfig
	stopCh       chan struct{}
	client       *http.Client
}

func NewEventHandler(idx *SpecIndex, rc *ResultClient, cfg pluginConfig) *EventHandler {
	return &EventHandler{
		specIndex:    idx,
		resultClient: rc,
		config:       cfg,
		stopCh:       make(chan struct{}),
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// Start begins the background polling loop.
func (eh *EventHandler) Start() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	slog.Info("event handler started", "module", moduleID, "poll_interval", "30s")

	for {
		select {
		case <-eh.stopCh:
			slog.Info("event handler stopped", "module", moduleID)
			return
		case <-ticker.C:
			eh.poll()
		}
	}
}

func (eh *EventHandler) Stop() {
	close(eh.stopCh)
}

func (eh *EventHandler) poll() {
	// Check MDEMG distribution endpoint for new observations
	url := fmt.Sprintf("%s/v1/memory/distribution?space_id=%s", eh.config.mdemgEndpoint, eh.config.spaceID)
	resp, err := eh.client.Get(url)
	if err != nil {
		return // Silent failure — server may be unavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var dist struct {
		Stats struct {
			ObservationCount int `json:"observation_count"`
		} `json:"stats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dist); err != nil {
		return
	}

	// Refresh spec index if observations have changed
	// (Simple approach: just reload specs periodically)
	if err := eh.specIndex.Load(eh.config.specRoot, eh.config.frameworks, eh.config.enableDrift); err != nil {
		slog.Error("spec index refresh error", "module", moduleID, "error", err)
		return
	}

	// Report any drift
	if eh.config.enableDrift && eh.specIndex.DriftCount() > 0 {
		stats := eh.specIndex.FrameworkStats()
		for fw := range stats {
			drifted := eh.findDriftedSpecs(fw)
			if len(drifted) > 0 {
				cleanCount := stats[fw] - len(drifted)
				eh.resultClient.StoreDriftResults(fw, drifted, cleanCount)
			}
		}
	}
}

func (eh *EventHandler) findDriftedSpecs(framework string) []string {
	eh.specIndex.mu.RLock()
	defer eh.specIndex.mu.RUnlock()

	var drifted []string
	for _, entry := range eh.specIndex.byFramework[framework] {
		if entry.DriftState == "drift_detected" {
			drifted = append(drifted, entry.FileName)
		}
	}
	return drifted
}
