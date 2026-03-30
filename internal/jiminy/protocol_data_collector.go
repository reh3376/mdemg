package jiminy

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// protocolTrainingRecord is a single JSONL line capturing one protocol event.
type protocolTrainingRecord struct {
	Timestamp          string  `json:"timestamp"`
	ConstraintCode     string  `json:"constraint_code"`
	ConstraintNodeID   string  `json:"constraint_node_id"`
	ConstraintText     string  `json:"constraint_text"`
	TierUsed           int     `json:"tier_used"`
	TokenCount         int     `json:"token_count"`
	AgentAction        string  `json:"agent_action"`
	Outcome            string  `json:"outcome"`
	ComprehensionScore float64 `json:"comprehension_score"`
	TrustScore         float64 `json:"trust_score"`
	SessionID          string  `json:"session_id"`
	// NS-03: Expanded fields for sidecar arbitration training
	ApplicabilityScore float64 `json:"applicability_score,omitempty"`
	ScoreSource        string  `json:"score_source,omitempty"`    // "heuristic" or "nli"
	MLTier             int     `json:"ml_tier,omitempty"`         // ML-predicted tier (0 if unavailable)
	RuleTier           int     `json:"rule_tier,omitempty"`       // Rule-based tier
	MLConfidence       float64 `json:"ml_confidence,omitempty"`   // ML prediction confidence
	TierSource         string  `json:"tier_source,omitempty"`     // "rule", "ml", or "fallback"
	SidecarMode        string  `json:"sidecar_mode,omitempty"`    // shadow/compare/canary/active
	GuidanceID         string  `json:"guidance_id,omitempty"`     // Correlation key for llm_interactions join
}

// ProtocolDataCollector writes protocol events to JSONL files for ML training.
type ProtocolDataCollector struct {
	mu         sync.Mutex
	dir        string
	maxFileSize int64
	file       *os.File
	written    int64
}

// NewProtocolDataCollector creates a new protocol data collector.
func NewProtocolDataCollector(dir string) *ProtocolDataCollector {
	if err := os.MkdirAll(dir, 0700); err != nil {
		slog.Error("j17: protocol data collector: failed to create dir", "dir", dir, "error", err)
	}
	return &ProtocolDataCollector{
		dir:         dir,
		maxFileSize: 50 * 1024 * 1024, // 50MB
	}
}

// Collect records a protocol event asynchronously.
func (dc *ProtocolDataCollector) Collect(rec protocolTrainingRecord) {
	go dc.writeRecord(rec)
}

func (dc *ProtocolDataCollector) writeRecord(rec protocolTrainingRecord) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	rec.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)

	data, err := json.Marshal(rec)
	if err != nil {
		slog.Error("j17: protocol data collector: marshal error", "error", err)
		return
	}
	data = append(data, '\n')

	// Rotate file if needed
	if dc.file == nil || dc.written >= dc.maxFileSize {
		if dc.file != nil {
			dc.file.Close()
		}
		filename := filepath.Join(dc.dir, "protocol-"+time.Now().UTC().Format("20060102-150405")+".jsonl")
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			slog.Error("j17: protocol data collector: open error", "error", err)
			return
		}
		dc.file = f
		dc.written = 0
	}

	n, err := dc.file.Write(data)
	if err != nil {
		slog.Error("j17: protocol data collector: write error", "error", err)
		return
	}
	dc.written += int64(n)
}

// Close closes the current file handle.
func (dc *ProtocolDataCollector) Close() {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if dc.file != nil {
		dc.file.Close()
		dc.file = nil
	}
}
