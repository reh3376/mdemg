// HOOKSYNC-001 — hook-channel heartbeat endpoint.
//
// Claude Code hooks POST one row per fire (prompt-context) or per throttled
// heartbeat (post-tool-observe) so the alert evaluator can detect the
// channel-silent condition: sessions demonstrably active (post-tool-observe
// rows present) while the per-prompt channel records nothing. Rows land in
// the V0024 scheduled_job_events hypertable via the jobhealth policy point,
// as job_name "hook:<name>" — no new sink (the EVENTGRAPH-002/-004 rule).
package api

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/jackc/pgx/v5/pgxpool"

	"mdemg/internal/jobhealth"
	"mdemg/internal/tsdb"
)

// hookEventRequest is the body for POST /v1/hooks/event.
type hookEventRequest struct {
	Hook       string `json:"hook"`                  // e.g. "prompt-context" — becomes job_name "hook:<hook>"
	SessionID  string `json:"session_id,omitempty"`  // per-conversation id
	DurationMS int64  `json:"duration_ms,omitempty"` // hook wall-clock, best-effort
	OK         *bool  `json:"ok,omitempty"`          // default true; false records a failure row
	Error      string `json:"error,omitempty"`       // failure detail when ok=false
}

// hookNameRe bounds hook names to a safe job_name suffix.
var hookNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// handleHookEvent processes POST /v1/hooks/event. Nil-safe on both the TSDB
// pool and the dispatcher (jobhealth policy) — a degraded server still
// accepts the heartbeat without erroring the hook.
func (s *Server) handleHookEvent(w http.ResponseWriter, r *http.Request) {
	var req hookEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if !hookNameRe.MatchString(req.Hook) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hook must match ^[a-z0-9][a-z0-9-]{0,63}$"})
		return
	}

	ok := true
	if req.OK != nil {
		ok = *req.OK
	}

	ev := tsdb.JobEventRow{
		JobName:    "hook:" + req.Hook,
		InstanceID: s.cfg.InstanceID,
		Success:    ok,
		LatencyMS:  req.DurationMS,
	}
	if !ok {
		ev.ErrorMessage = req.Error
	}
	if req.SessionID != "" {
		ev.Metadata = map[string]any{"session_id": req.SessionID}
	}

	var pool *pgxpool.Pool
	if s.tsdbClient != nil {
		pool = s.tsdbClient.Pool()
	}
	jobhealth.Report(r.Context(), pool, s.alertDispatcher, ev)
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}
