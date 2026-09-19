// Sprint JIMINY-PROCESS-OBSERVER-01 (task #160) Epic 3 — process-event ingest.
//
// POST /v1/process/event accepts one batch of client-side observed process
// events (from .claude/hooks/post-tool-observe.py or any future emitter),
// scrubs text fields for privacy, and enqueues to the buffered V0036
// writer. Fire-and-forget contract: returns 202 immediately, never blocks
// on the writer. When PROCESS_EVENTS_ENABLED=false the endpoint returns
// 503 with a "flip the flag" hint so operators know why nothing landed.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"mdemg/internal/llmclient"
	"mdemg/internal/tsdb"
)

// ProcessEventInput is one client-supplied event. Time is optional (server
// clamps to NOW() when zero); ExitCode is optional (pointer so 0 vs unset is
// distinguishable). Metadata is free-form per event type.
type ProcessEventInput struct {
	SpaceID      string         `json:"space_id"`
	SessionID    string         `json:"session_id"`
	EventType    string         `json:"event_type"`
	EventSubtype string         `json:"event_subtype,omitempty"`
	Outcome      string         `json:"outcome,omitempty"`      // success | failure | unknown
	ExitCode     *int           `json:"exit_code,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	SourceHook   string         `json:"source_hook,omitempty"`
	TimeMillis   int64          `json:"time_millis,omitempty"` // client-supplied event time (unix millis); 0 → server NOW()
}

// ProcessEventBatch is the request body — one call may enqueue N events.
type ProcessEventBatch struct {
	Events []ProcessEventInput `json:"events"`
}

// Allowed event_type / outcome enums. Reject anything else with 400 so the
// TSDB rows stay clean and future matchers can trust the enum.
var (
	allowedProcessEventTypes = map[string]struct{}{
		"lint_run":         {},
		"model_call":       {},
		"mcp_call":         {},
		"retrieval_call":   {},
		"bash_command":     {},
		"git_commit":       {},
		"git_push":         {},
		"test_run":         {},
		"plan_mode_entry":  {},
		"file_write":       {},
		"filesystem_search": {}, // JIMINY-PROCESS-OBSERVER-03
	}
	allowedProcessEventOutcomes = map[string]struct{}{
		"":        {}, // treated as "unknown" by writer
		"unknown": {},
		"success": {},
		"failure": {},
	}
)

// handleProcessEvent — POST /v1/process/event
func (s *Server) handleProcessEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if s.processEventsWriter == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "process events ingest disabled (flip PROCESS_EVENTS_ENABLED=true in .env + restart)",
		})
		return
	}
	var batch ProcessEventBatch
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if len(batch.Events) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "events array is required (min 1)"})
		return
	}

	accepted := 0
	rejected := make([]string, 0)
	for i, ev := range batch.Events {
		if ev.SpaceID == "" {
			rejected = append(rejected, indexedReason(i, "space_id required"))
			continue
		}
		if ev.SessionID == "" {
			rejected = append(rejected, indexedReason(i, "session_id required"))
			continue
		}
		if _, ok := allowedProcessEventTypes[ev.EventType]; !ok {
			rejected = append(rejected, indexedReason(i, "unknown event_type: "+ev.EventType))
			continue
		}
		if _, ok := allowedProcessEventOutcomes[ev.Outcome]; !ok {
			rejected = append(rejected, indexedReason(i, "unknown outcome: "+ev.Outcome))
			continue
		}

		row := tsdb.ProcessEventRow{
			SpaceID:      ev.SpaceID,
			SessionID:    ev.SessionID,
			EventType:    ev.EventType,
			EventSubtype: llmclient.ScrubString(ev.EventSubtype),
			Outcome:      ev.Outcome,
			ExitCode:     ev.ExitCode,
			SourceHook:   ev.SourceHook,
		}
		if ev.TimeMillis > 0 {
			row.Time = time.UnixMilli(ev.TimeMillis)
		}
		if len(ev.Metadata) > 0 {
			row.Metadata = scrubMetadata(ev.Metadata)
		}
		s.processEventsWriter.Record(row)
		accepted++
	}

	status := http.StatusAccepted
	if accepted == 0 && len(rejected) > 0 {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]any{
		"accepted": accepted,
		"rejected": rejected,
	})
}

// scrubMetadata applies llmclient.ScrubString to string values in the
// metadata map — non-string values pass through untouched. Nested maps are
// walked one level (defensive shallow scrub; PII-shaped strings are
// virtually always top-level or one level deep in the hook payload).
func scrubMetadata(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch typed := v.(type) {
		case string:
			out[k] = llmclient.ScrubString(typed)
		case map[string]any:
			inner := make(map[string]any, len(typed))
			for ik, iv := range typed {
				if s, ok := iv.(string); ok {
					inner[ik] = llmclient.ScrubString(s)
				} else {
					inner[ik] = iv
				}
			}
			out[k] = inner
		default:
			out[k] = v
		}
	}
	return out
}

func indexedReason(i int, reason string) string {
	return "event[" + itoaSmall(i) + "]: " + reason
}

// itoaSmall — tiny fmt-free itoa for small non-negative indexes (≤ 999).
// Avoids importing strconv/fmt for one hot call site. Falls back to "?" for
// values outside [0, 999] which is a defensive guard.
func itoaSmall(n int) string {
	if n < 0 || n > 999 {
		return "?"
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	if n < 100 {
		return string([]byte{byte('0' + n/10), byte('0' + n%10)})
	}
	return string([]byte{byte('0' + n/100), byte('0' + (n/10)%10), byte('0' + n%10)})
}
