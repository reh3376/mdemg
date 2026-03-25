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
	"strings"
	"sync"
	"time"

	pb "mdemg/api/modulepb"
	"mdemg/internal/models"
)

// WebhookConfig holds configuration for a webhook source.
type WebhookConfig struct {
	Source        string // e.g., "github", "gitlab", "bitbucket"
	Secret        string // HMAC signing secret
	SpaceID       string // Target space for observations
	ModuleID      string // Ingestion module to use (optional)
	SignatureHeader string // Header containing signature (default varies by source)
}

// webhookDebouncer coalesces rapid webhook events for the same entity.
type webhookDebouncer struct {
	mu     sync.Mutex
	timers map[string]*time.Timer // "source:entity_id" -> timer
}

func newWebhookDebouncer() *webhookDebouncer {
	return &webhookDebouncer{
		timers: make(map[string]*time.Timer),
	}
}

// debounce schedules fn to run after a 10s quiet period for the given key.
func (d *webhookDebouncer) debounce(key string, fn func()) {
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

// genericWebhookPayload represents a normalized webhook payload.
type genericWebhookPayload struct {
	Source      string          `json:"source"`       // github, gitlab, etc.
	Action      string          `json:"action"`       // push, pull_request, issue, etc.
	EventType   string          `json:"event_type"`   // create, update, delete
	EntityID    string          `json:"entity_id"`    // Unique ID for the entity
	EntityType  string          `json:"entity_type"`  // repository, issue, pr, etc.
	URL         string          `json:"url"`          // URL to the entity
	Timestamp   string          `json:"timestamp"`    // Event timestamp
	RawPayload  json.RawMessage `json:"raw_payload"`  // Original payload
	Metadata    map[string]string `json:"metadata"`   // Additional metadata
}

// handleGenericWebhook handles POST /v1/webhooks/{source}
// Routes to appropriate ingestion module based on source config.
func (s *Server) handleGenericWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Extract source from URL path: /v1/webhooks/{source}
	path := strings.TrimPrefix(r.URL.Path, "/v1/webhooks/")
	source := strings.TrimSuffix(path, "/")

	// Skip Linear - it has its own dedicated handler
	if source == "linear" {
		s.handleLinearWebhook(w, r)
		return
	}

	if source == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "source is required in path"})
		return
	}

	// Get config for this source
	config := s.getWebhookConfig(source)
	if config == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("webhook source %q not configured", source)})
		return
	}

	// Read body (limit 1MB)
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to read body"})
		return
	}

	// Verify signature if secret is configured
	if config.Secret != "" {
		signature := getWebhookSignature(r, config)
		if signature == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "missing signature"})
			return
		}

		if !verifyWebhookSignature(body, signature, config.Secret, source) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid signature"})
			return
		}
	}

	// Parse and normalize the payload
	payload, err := parseWebhookPayload(source, body, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("failed to parse payload: %v", err)})
		return
	}
	payload.Source = source

	// Debounce by source:entity_id
	debounceKey := fmt.Sprintf("%s:%s", source, payload.EntityID)

	// Capture for goroutine
	payloadCopy := *payload
	bodyCopy := make([]byte, len(body))
	copy(bodyCopy, body)
	configCopy := *config

	s.genericWebhookDebouncer.debounce(debounceKey, func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("webhook: panic recovered", "source", source, "panic", r)
			}
		}()
		s.processGenericWebhookEvent(payloadCopy, bodyCopy, configCopy)
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "accepted",
		"source":      source,
		"entity_type": payload.EntityType,
		"action":      payload.Action,
		"debounce":    debounceKey,
	})
}

// getWebhookConfig returns the webhook configuration for a source.
func (s *Server) getWebhookConfig(source string) *WebhookConfig {
	// Parse from WEBHOOK_CONFIGS environment variable
	// Format: "source:secret:space_id,source2:secret2:space_id2"
	configs := s.parseWebhookConfigs()
	if config, ok := configs[source]; ok {
		return &config
	}
	return nil
}

// parseWebhookConfigs parses the webhook configurations from config.
func (s *Server) parseWebhookConfigs() map[string]WebhookConfig {
	configs := make(map[string]WebhookConfig)

	// This would typically come from s.cfg.WebhookConfigs
	// For now, return empty map if not configured
	if s.cfg.WebhookConfigs == "" {
		return configs
	}

	for _, item := range strings.Split(s.cfg.WebhookConfigs, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), ":", 3)
		if len(parts) >= 2 {
			config := WebhookConfig{
				Source: parts[0],
				Secret: parts[1],
			}
			if len(parts) >= 3 {
				config.SpaceID = parts[2]
			}
			configs[parts[0]] = config
		}
	}

	return configs
}

// getWebhookSignature extracts the signature from the request based on source.
func getWebhookSignature(r *http.Request, config *WebhookConfig) string {
	if config.SignatureHeader != "" {
		return r.Header.Get(config.SignatureHeader)
	}

	// Default signature headers by source
	switch config.Source {
	case "github":
		return strings.TrimPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
	case "gitlab":
		return r.Header.Get("X-Gitlab-Token")
	case "bitbucket":
		return r.Header.Get("X-Hub-Signature")
	default:
		// Try common headers
		if sig := r.Header.Get("X-Signature"); sig != "" {
			return sig
		}
		if sig := r.Header.Get("X-Webhook-Signature"); sig != "" {
			return sig
		}
		return ""
	}
}

// verifyWebhookSignature verifies the webhook signature based on source.
func verifyWebhookSignature(body []byte, signature, secret, source string) bool {
	switch source {
	case "github":
		// GitHub uses HMAC-SHA256
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		return hmac.Equal([]byte(expected), []byte(signature))

	case "gitlab":
		// GitLab uses a simple token comparison
		return hmac.Equal([]byte(secret), []byte(signature))

	case "bitbucket":
		// Bitbucket uses HMAC-SHA256
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		return hmac.Equal([]byte(expected), []byte(signature))

	default:
		// Default: HMAC-SHA256
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		return hmac.Equal([]byte(expected), []byte(signature))
	}
}

// parseWebhookPayload parses the raw webhook payload into a normalized format.
func parseWebhookPayload(source string, body []byte, r *http.Request) (*genericWebhookPayload, error) {
	payload := &genericWebhookPayload{
		RawPayload: body,
		Metadata:   make(map[string]string),
	}

	switch source {
	case "github":
		return parseGitHubWebhook(body, r, payload)
	case "gitlab":
		return parseGitLabWebhook(body, r, payload)
	case "bitbucket":
		return parseBitbucketWebhook(body, r, payload)
	default:
		// Generic JSON parsing
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}

		// Try to extract common fields
		if id, ok := data["id"].(string); ok {
			payload.EntityID = id
		} else if id, ok := data["id"].(float64); ok {
			payload.EntityID = fmt.Sprintf("%.0f", id)
		}

		if action, ok := data["action"].(string); ok {
			payload.Action = action
		}
		if eventType, ok := data["event"].(string); ok {
			payload.EventType = eventType
		}
		if url, ok := data["url"].(string); ok {
			payload.URL = url
		}

		payload.Timestamp = time.Now().UTC().Format(time.RFC3339)
		return payload, nil
	}
}

// parseGitHubWebhook parses a GitHub webhook payload.
func parseGitHubWebhook(body []byte, r *http.Request, payload *genericWebhookPayload) (*genericWebhookPayload, error) {
	eventType := r.Header.Get("X-GitHub-Event")
	payload.EventType = eventType

	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Determine action and entity based on event type
	if action, ok := data["action"].(string); ok {
		payload.Action = action
	}

	switch eventType {
	case "push":
		payload.EntityType = "push"
		if ref, ok := data["ref"].(string); ok {
			payload.EntityID = ref
		}
		if headCommit, ok := data["head_commit"].(map[string]any); ok {
			if id, ok := headCommit["id"].(string); ok {
				payload.EntityID = id
				payload.Metadata["commit_sha"] = id
			}
			if url, ok := headCommit["url"].(string); ok {
				payload.URL = url
			}
		}

	case "pull_request":
		payload.EntityType = "pull_request"
		if pr, ok := data["pull_request"].(map[string]any); ok {
			if id, ok := pr["id"].(float64); ok {
				payload.EntityID = fmt.Sprintf("%.0f", id)
			}
			if url, ok := pr["html_url"].(string); ok {
				payload.URL = url
			}
		}

	case "issues":
		payload.EntityType = "issue"
		if issue, ok := data["issue"].(map[string]any); ok {
			if id, ok := issue["id"].(float64); ok {
				payload.EntityID = fmt.Sprintf("%.0f", id)
			}
			if url, ok := issue["html_url"].(string); ok {
				payload.URL = url
			}
		}

	default:
		payload.EntityType = eventType
		if id, ok := data["id"].(float64); ok {
			payload.EntityID = fmt.Sprintf("%.0f", id)
		}
	}

	// Extract repository info
	if repo, ok := data["repository"].(map[string]any); ok {
		if fullName, ok := repo["full_name"].(string); ok {
			payload.Metadata["repository"] = fullName
		}
	}

	payload.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return payload, nil
}

// parseGitLabWebhook parses a GitLab webhook payload.
func parseGitLabWebhook(body []byte, r *http.Request, payload *genericWebhookPayload) (*genericWebhookPayload, error) {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	eventType, _ := data["object_kind"].(string)
	payload.EventType = eventType
	payload.EntityType = eventType

	switch eventType {
	case "push":
		if checkout, ok := data["checkout_sha"].(string); ok {
			payload.EntityID = checkout
			payload.Metadata["commit_sha"] = checkout
		}

	case "merge_request":
		if attrs, ok := data["object_attributes"].(map[string]any); ok {
			if id, ok := attrs["id"].(float64); ok {
				payload.EntityID = fmt.Sprintf("%.0f", id)
			}
			if url, ok := attrs["url"].(string); ok {
				payload.URL = url
			}
			if action, ok := attrs["action"].(string); ok {
				payload.Action = action
			}
		}

	case "issue":
		if attrs, ok := data["object_attributes"].(map[string]any); ok {
			if id, ok := attrs["id"].(float64); ok {
				payload.EntityID = fmt.Sprintf("%.0f", id)
			}
			if url, ok := attrs["url"].(string); ok {
				payload.URL = url
			}
			if action, ok := attrs["action"].(string); ok {
				payload.Action = action
			}
		}
	}

	// Extract project info
	if project, ok := data["project"].(map[string]any); ok {
		if name, ok := project["path_with_namespace"].(string); ok {
			payload.Metadata["repository"] = name
		}
	}

	payload.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return payload, nil
}

// parseBitbucketWebhook parses a Bitbucket webhook payload.
func parseBitbucketWebhook(body []byte, r *http.Request, payload *genericWebhookPayload) (*genericWebhookPayload, error) {
	eventType := r.Header.Get("X-Event-Key")
	payload.EventType = eventType

	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	switch {
	case strings.HasPrefix(eventType, "repo:push"):
		payload.EntityType = "push"
		if push, ok := data["push"].(map[string]any); ok {
			if changes, ok := push["changes"].([]any); ok && len(changes) > 0 {
				if change, ok := changes[0].(map[string]any); ok {
					if newCommit, ok := change["new"].(map[string]any); ok {
						if target, ok := newCommit["target"].(map[string]any); ok {
							if hash, ok := target["hash"].(string); ok {
								payload.EntityID = hash
								payload.Metadata["commit_sha"] = hash
							}
						}
					}
				}
			}
		}

	case strings.HasPrefix(eventType, "pullrequest"):
		payload.EntityType = "pull_request"
		if pr, ok := data["pullrequest"].(map[string]any); ok {
			if id, ok := pr["id"].(float64); ok {
				payload.EntityID = fmt.Sprintf("%.0f", id)
			}
			if links, ok := pr["links"].(map[string]any); ok {
				if html, ok := links["html"].(map[string]any); ok {
					if href, ok := html["href"].(string); ok {
						payload.URL = href
					}
				}
			}
		}
	}

	// Extract repository info
	if repo, ok := data["repository"].(map[string]any); ok {
		if fullName, ok := repo["full_name"].(string); ok {
			payload.Metadata["repository"] = fullName
		}
	}

	payload.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return payload, nil
}

// processGenericWebhookEvent processes a debounced generic webhook event.
func (s *Server) processGenericWebhookEvent(payload genericWebhookPayload, rawBody []byte, config WebhookConfig) {
	slog.Info("webhook: processing event", "source", payload.Source, "entity_type", payload.EntityType, "action", payload.Action, "entity_id", payload.EntityID)

	// Determine space ID
	spaceID := config.SpaceID
	if spaceID == "" {
		slog.Warn("webhook: no space ID configured", "source", payload.Source)
		return
	}

	// Try to find a matching ingestion module
	var moduleInfo any
	if s.pluginMgr != nil && config.ModuleID != "" {
		if mod, ok := s.pluginMgr.GetModule(config.ModuleID); ok {
			moduleInfo = mod
		}
	}

	var items []models.BatchIngestItem

	if moduleInfo != nil {
		// Use ingestion module to parse
		items = s.parseWithIngestionModule(payload, rawBody, config)
	} else {
		// Default: create a single observation from the webhook event
		items = []models.BatchIngestItem{{
			Timestamp: payload.Timestamp,
			Source:    fmt.Sprintf("%s-webhook", payload.Source),
			Content:   string(rawBody),
			Tags:      []string{payload.Source, payload.EntityType, payload.Action},
			NodeID:    fmt.Sprintf("%s-%s-%s", payload.Source, payload.EntityType, payload.EntityID),
			Path:      fmt.Sprintf("webhooks/%s/%s", payload.Source, payload.EntityType),
			Name:      fmt.Sprintf("%s %s: %s", payload.Source, payload.EntityType, payload.EntityID),
		}}
	}

	if len(items) == 0 {
		slog.Warn("webhook: no observations to ingest", "source", payload.Source)
		return
	}

	// Batch ingest
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := s.retriever.BatchIngestObservations(ctx, models.BatchIngestRequest{
		SpaceID:      spaceID,
		Observations: items,
	})
	if err != nil {
		slog.Error("webhook: batch ingest failed", "source", payload.Source, "error", err)
		return
	}

	slog.Info("webhook: ingested observations", "source", payload.Source, "success_count", resp.SuccessCount, "total_items", resp.TotalItems)

	// Update TapRoot freshness
	if err := s.retriever.UpdateTapRootFreshness(ctx, spaceID, fmt.Sprintf("%s-webhook", payload.Source), IsPrunablePrefix(spaceID)); err != nil {
		slog.Error("webhook: failed to update TapRoot freshness", "source", payload.Source, "error", err)
	}

	// Trigger APE events
	s.TriggerAPEEventWithContext("source_changed", map[string]string{
		"space_id":    spaceID,
		"ingest_type": fmt.Sprintf("%s-webhook", payload.Source),
		"entity_type": payload.EntityType,
		"action":      payload.Action,
	})
}

// parseWithIngestionModule uses an ingestion module to parse the webhook payload.
func (s *Server) parseWithIngestionModule(payload genericWebhookPayload, rawBody []byte, config WebhookConfig) []models.BatchIngestItem {
	if s.pluginMgr == nil {
		return nil
	}

	modInfo, ok := s.pluginMgr.GetModule(config.ModuleID)
	if !ok || modInfo.IngestionClient == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	contentType := fmt.Sprintf("application/vnd.%s.%s", payload.Source, payload.EntityType)

	parseResp, err := modInfo.IngestionClient.Parse(ctx, &pb.ParseRequest{
		SourceUri:   payload.URL,
		ContentType: contentType,
		Content:     rawBody,
		Metadata: map[string]string{
			"source":      payload.Source,
			"action":      payload.Action,
			"event_type":  payload.EventType,
			"entity_type": payload.EntityType,
			"entity_id":   payload.EntityID,
		},
	})

	if err != nil {
		slog.Error("webhook: module parse failed", "source", payload.Source, "error", err)
		return nil
	}
	if parseResp.Error != "" {
		slog.Error("webhook: module parse error", "source", payload.Source, "error", parseResp.Error)
		return nil
	}

	items := make([]models.BatchIngestItem, 0, len(parseResp.Observations))
	for _, obs := range parseResp.Observations {
		ts := obs.Timestamp
		if ts == "" {
			ts = time.Now().UTC().Format(time.RFC3339)
		}
		source := obs.Source
		if source == "" {
			source = fmt.Sprintf("%s-webhook", payload.Source)
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

	return items
}

// genericWebhookDebouncer is the server's debouncer for generic webhooks.
// Initialized in NewServer.
var _ = func() int {
	// This ensures the debouncer field is added to Server
	return 0
}()
