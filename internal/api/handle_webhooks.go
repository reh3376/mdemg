package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	pb "mdemg/api/modulepb"
	"mdemg/internal/models"
)

// linearWebhookDebouncer coalesces rapid webhook events for the same entity
// to avoid redundant processing (e.g., multiple rapid field updates on one issue).
type linearWebhookDebouncer struct {
	mu     sync.Mutex
	timers map[string]*time.Timer // "entity_type:entity_id" → timer
}

func newLinearWebhookDebouncer() *linearWebhookDebouncer {
	return &linearWebhookDebouncer{
		timers: make(map[string]*time.Timer),
	}
}

// debounce schedules fn to run after a 10s quiet period for the given key.
// If called again for the same key within 10s, the timer resets.
func (d *linearWebhookDebouncer) debounce(key string, fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if timer, ok := d.timers[key]; ok {
		timer.Stop()
	}

	d.timers[key] = time.AfterFunc(10*time.Second, func() {
		d.mu.Lock()
		delete(d.timers, key)
		d.mu.Unlock()
		fn()
	})
}

// linearWebhookPayload represents the top-level webhook payload from Linear.
type linearWebhookPayload struct {
	Action         string          `json:"action"`         // "create", "update", "remove"
	Type           string          `json:"type"`           // "Issue", "Project", "Comment"
	Data           json.RawMessage `json:"data"`           // Entity data
	URL            string          `json:"url"`            // Linear URL for the entity
	CreatedAt      string          `json:"createdAt"`      // Webhook event timestamp
	OrganizationId string          `json:"organizationId"` // Linear organization ID
}

// linearEntityData is a minimal extraction of entity fields for ID and title.
type linearEntityData struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// handleLinearWebhook handles POST /v1/webhooks/linear
func (s *Server) handleLinearWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Check that webhook secret is configured
	if s.cfg.LinearWebhookSecret == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "webhook secret not configured"})
		return
	}

	// Read body (limit 1MB)
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to read body"})
		return
	}

	// Verify HMAC-SHA256 signature
	signature := r.Header.Get("Linear-Signature")
	if signature == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing signature"})
		return
	}

	if !verifyLinearSignature(body, signature, s.cfg.LinearWebhookSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid signature"})
		return
	}

	// Parse payload
	var payload linearWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON payload"})
		return
	}

	// Filter: only process Issue (create/update) and Project (update)
	switch {
	case payload.Type == "Issue" && (payload.Action == "create" || payload.Action == "update"):
		// Process
	case payload.Type == "Project" && payload.Action == "update":
		// Process
	default:
		// Ignore other event types with 200 OK
		writeJSON(w, http.StatusOK, map[string]any{"status": "ignored", "type": payload.Type, "action": payload.Action})
		return
	}

	// Extract entity ID from Data
	var entity linearEntityData
	if err := json.Unmarshal(payload.Data, &entity); err != nil || entity.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing entity ID in data"})
		return
	}

	// Debounce: 10s window per type:id
	debounceKey := fmt.Sprintf("%s:%s", payload.Type, entity.ID)

	// Capture payload fields for the goroutine
	payloadCopy := payload
	bodyCopy := make([]byte, len(body))
	copy(bodyCopy, body)

	s.webhookDebouncer.debounce(debounceKey, func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("webhook linear: panic recovered in processLinearWebhookEvent", "panic", r)
			}
		}()
		s.processLinearWebhookEvent(payloadCopy, bodyCopy)
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "accepted",
		"type":     payload.Type,
		"action":   payload.Action,
		"debounce": debounceKey,
	})
}

// processLinearWebhookEvent processes a debounced Linear webhook event.
func (s *Server) processLinearWebhookEvent(payload linearWebhookPayload, rawBody []byte) {
	if s.pluginMgr == nil {
		slog.Warn("webhook linear: plugin manager not available")
		return
	}

	// Find Linear module
	modInfo, ok := s.pluginMgr.GetModule("linear-module")
	if !ok || modInfo.IngestionClient == nil {
		slog.Warn("webhook linear: linear-module not found or not ingestion type")
		return
	}

	// Map Type to content_type
	var contentType string
	switch payload.Type {
	case "Issue":
		contentType = "application/vnd.linear.issue"
	case "Project":
		contentType = "application/vnd.linear.project"
	default:
		contentType = "application/vnd.linear." + payload.Type
	}

	// Call module's Parse method
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	parseResp, err := modInfo.IngestionClient.Parse(ctx, &pb.ParseRequest{
		SourceUri:   payload.URL,
		ContentType: contentType,
		Content:     payload.Data,
		Metadata: map[string]string{
			"action":          payload.Action,
			"type":            payload.Type,
			"organization_id": payload.OrganizationId,
			"created_at":      payload.CreatedAt,
		},
	})

	if err != nil {
		slog.Error("webhook linear: parse failed", "error", err)
		return
	}
	if parseResp.Error != "" {
		slog.Error("webhook linear: parse error", "error", parseResp.Error)
		return
	}

	if len(parseResp.Observations) == 0 {
		slog.Warn("webhook linear: no observations returned from parse")
		return
	}

	// Determine space ID
	spaceID := s.cfg.LinearWebhookSpaceID
	if spaceID == "" {
		slog.Warn("webhook linear: no space ID configured (LINEAR_WEBHOOK_SPACE_ID)")
		return
	}

	// Convert observations to batch ingest items
	items := make([]models.BatchIngestItem, 0, len(parseResp.Observations))
	for _, obs := range parseResp.Observations {
		ts := obs.Timestamp
		if ts == "" {
			ts = time.Now().UTC().Format(time.RFC3339)
		}
		source := obs.Source
		if source == "" {
			source = "linear-webhook"
		}
		items = append(items, models.BatchIngestItem{
			Timestamp: ts,
			Source:    source,
			Content:   obs.Content,
			Tags:      obs.Tags,
			NodeID:    obs.NodeId,
			Path:      obs.Path,
			Name:      obs.Name,
		})
	}

	// Batch ingest
	ingestCtx, ingestCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer ingestCancel()

	resp, err := s.retriever.BatchIngestObservations(ingestCtx, models.BatchIngestRequest{
		SpaceID:      spaceID,
		Observations: items,
	})
	if err != nil {
		slog.Error("webhook linear: batch ingest failed", "error", err)
		return
	}

	slog.Info("webhook linear: ingested observations", "success_count", resp.SuccessCount, "total_items", resp.TotalItems, "type", payload.Type, "action", payload.Action)

	// Update TapRoot freshness
	if err := s.retriever.UpdateTapRootFreshness(ingestCtx, spaceID, "linear-webhook", IsPrunablePrefix(spaceID)); err != nil {
		slog.Error("webhook linear: failed to update TapRoot freshness", "error", err)
	}

	// Trigger APE events
	s.TriggerAPEEventWithContext("source_changed", map[string]string{
		"space_id":    spaceID,
		"ingest_type": "linear-webhook",
		"entity_type": payload.Type,
		"action":      payload.Action,
	})
}

// verifyLinearSignature verifies the HMAC-SHA256 signature from Linear webhooks.
func verifyLinearSignature(body []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
