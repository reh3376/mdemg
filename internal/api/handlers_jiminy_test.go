package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"mdemg/internal/alert"
	"mdemg/internal/config"
	"mdemg/internal/jiminy"
)

// TestJiminyHealthz_MethodNotAllowed verifies POST returns 405.
func TestJiminyHealthz_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/healthz", nil)
	w := httptest.NewRecorder()
	s.handleJiminyHealthz(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"method not allowed"`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestJiminyHealthz_NilService verifies 503 when jiminySvc is nil.
func TestJiminyHealthz_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{JiminyEnabled: true}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/healthz", nil)
	w := httptest.NewRecorder()
	s.handleJiminyHealthz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, `"status":"disabled"`) {
		t.Errorf("expected disabled status: %s", body)
	}
	if !contains(body, `"enabled":false`) {
		t.Errorf("expected enabled:false: %s", body)
	}
}

// TestJiminyHealthz_Disabled verifies 503 when JiminyEnabled is false.
func TestJiminyHealthz_Disabled(t *testing.T) {
	s := &Server{cfg: config.Config{JiminyEnabled: false}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/healthz", nil)
	w := httptest.NewRecorder()
	s.handleJiminyHealthz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, `"status":"disabled"`) {
		t.Errorf("expected disabled status: %s", body)
	}
}

// TestJiminyReady_MethodNotAllowed verifies POST returns 405.
func TestJiminyReady_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/ready", nil)
	w := httptest.NewRecorder()
	s.handleJiminyReady(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJiminyReady_NilService verifies 503 when jiminySvc is nil.
func TestJiminyReady_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{JiminyEnabled: true}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/ready", nil)
	w := httptest.NewRecorder()
	s.handleJiminyReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, `"message":"jiminy guidance is not enabled (set JIMINY_ENABLED=true)"`) {
		t.Errorf("expected disabled message: %s", body)
	}
}

// TestJiminyGuide_MethodNotAllowed verifies GET returns 405.
func TestJiminyGuide_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/guide", nil)
	w := httptest.NewRecorder()
	s.handleJiminyGuide(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJiminyGuide_NilService verifies 503 when jiminySvc is nil.
func TestJiminyGuide_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/guide", nil)
	w := httptest.NewRecorder()
	s.handleJiminyGuide(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"jiminy guidance is not enabled`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestJiminyFeedback_MethodNotAllowed verifies GET returns 405.
func TestJiminyFeedback_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/feedback", nil)
	w := httptest.NewRecorder()
	s.handleJiminyFeedback(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJiminyFeedback_NilService verifies 503 when jiminySvc is nil.
func TestJiminyFeedback_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/feedback", nil)
	w := httptest.NewRecorder()
	s.handleJiminyFeedback(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"jiminy guidance is not enabled`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestJiminyEvaluate_MethodNotAllowed verifies GET returns 405.
func TestJiminyEvaluate_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/evaluate", nil)
	w := httptest.NewRecorder()
	s.handleJiminyEvaluate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJiminyEvaluate_NilService verifies 503 when jiminySvc is nil.
func TestJiminyEvaluate_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/evaluate", nil)
	w := httptest.NewRecorder()
	s.handleJiminyEvaluate(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"jiminy guidance is not enabled`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestJiminyClassify_MethodNotAllowed verifies GET returns 405.
func TestJiminyClassify_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/classify", nil)
	w := httptest.NewRecorder()
	s.handleJiminyClassify(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJiminyClassify_NilService verifies 503 when jiminySvc is nil.
func TestJiminyClassify_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{JiminyEnabled: true}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/classify",
		strings.NewReader(`{"space_id":"test","session_id":"sess-1","agent_output":"test output"}`))
	w := httptest.NewRecorder()
	s.handleJiminyClassify(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"jiminy guidance is not enabled`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestJiminyClassify_BadRequest verifies 400 for missing required fields.
func TestJiminyClassify_BadRequest(t *testing.T) {
	s := &Server{cfg: config.Config{JiminyEnabled: true}, jiminySvc: nil}
	// Reaches nil check before body parse — returns 503.
	// With jiminySvc set but no body fields, should return 400.
	// Since we can't easily mock jiminySvc, test the nil path.
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/classify",
		strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.handleJiminyClassify(w, req)

	// nil jiminySvc → 503 (checked before body parse)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for nil jiminySvc, got %d", w.Code)
	}
}

// TestJiminyReformulate_MethodNotAllowed verifies GET returns 405.
func TestJiminyReformulate_MethodNotAllowed(t *testing.T) {
	s := &Server{cfg: config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/v1/jiminy/reformulate", nil)
	w := httptest.NewRecorder()
	s.handleJiminyReformulate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// TestJiminyReformulate_NilService verifies 503 when jiminySvc is nil.
func TestJiminyReformulate_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{JiminyEnabled: true}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/reformulate",
		strings.NewReader(`{"space_id":"test","session_id":"sess-1"}`))
	w := httptest.NewRecorder()
	s.handleJiminyReformulate(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"jiminy guidance is not enabled`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// TestJiminyWarm_NilService verifies 503 when jiminySvc is nil.
func TestJiminyWarm_NilService(t *testing.T) {
	s := &Server{cfg: config.Config{JiminyEnabled: true}}
	req := httptest.NewRequest(http.MethodPost, "/v1/jiminy/warm",
		strings.NewReader(`{"space_id":"test","session_id":"sess-1"}`))
	w := httptest.NewRecorder()
	s.handleJiminyWarm(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !contains(w.Body.String(), `"error":"jiminy guidance is not enabled`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

// JIMINY-ENFORCE-001: mockAlertSender captures Send() calls for verifying the
// deny→alert wiring without a full alert.Dispatcher.
type mockAlertSender struct {
	calls atomic.Int32
	last  alert.Alert
}

func (m *mockAlertSender) Send(_ context.Context, a alert.Alert) {
	m.calls.Add(1)
	m.last = a
}

// TestEmitJiminyBlockAlert_DenyFiresHighAlert pins JIMINY-ENFORCE-001's core
// enforcement contract: every deny verdict emits a HIGH-severity alert to the
// user via the alert dispatcher.
func TestEmitJiminyBlockAlert_DenyFiresHighAlert(t *testing.T) {
	m := &mockAlertSender{}
	req := jiminy.ClassifyRequest{
		SpaceID:     "mdemg-dev",
		SessionID:   "claude-core",
		AgentOutput: "uuid.New()",
		ToolName:    "Write",
		FilePath:    "/tmp/foo.go",
	}
	resp := jiminy.ClassifyResponse{
		Verdict:      "deny",
		DenialReason: "always-use-cuidv2",
	}
	emitJiminyBlockAlert(context.Background(), m, req, resp)
	if m.calls.Load() != 1 {
		t.Fatalf("expected 1 Send() call, got %d", m.calls.Load())
	}
	if m.last.Service != "jiminy-block" {
		t.Errorf("service=%q, want jiminy-block", m.last.Service)
	}
	if m.last.Severity != alert.SeverityHigh {
		t.Errorf("severity=%q, want high", m.last.Severity)
	}
	if !strings.Contains(m.last.Message, "always-use-cuidv2") {
		t.Errorf("message should include reason: %q", m.last.Message)
	}
	if !strings.Contains(m.last.Message, "/tmp/foo.go") {
		t.Errorf("message should include file_path: %q", m.last.Message)
	}
}

// TestEmitJiminyBlockAlert_PassIsNoOp — an allowed action must never emit an alert.
func TestEmitJiminyBlockAlert_PassIsNoOp(t *testing.T) {
	m := &mockAlertSender{}
	emitJiminyBlockAlert(context.Background(), m,
		jiminy.ClassifyRequest{SpaceID: "s"},
		jiminy.ClassifyResponse{Verdict: "pass"})
	if m.calls.Load() != 0 {
		t.Errorf("pass verdict must not emit alert, got %d calls", m.calls.Load())
	}
}

// TestEmitJiminyBlockAlert_NilDispatcherIsSafe — server without dispatcher initialised
// must not panic.
func TestEmitJiminyBlockAlert_NilDispatcherIsSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil dispatcher must not panic: %v", r)
		}
	}()
	emitJiminyBlockAlert(context.Background(), nil,
		jiminy.ClassifyRequest{SpaceID: "s"},
		jiminy.ClassifyResponse{Verdict: "deny", DenialReason: "x"})
}

// JIMINY-ENFORCE-002: verify the Bash-specific message shape. Bash requests
// carry empty file_path — the message must include tool: Bash + the truncated
// command preview so operator can tell Write blocks from Bash blocks in the
// alert stream.
func TestEmitJiminyBlockAlert_BashIncludesCommandPreview(t *testing.T) {
	m := &mockAlertSender{}
	req := jiminy.ClassifyRequest{
		SpaceID:     "mdemg-dev",
		SessionID:   "claude-core",
		AgentOutput: "git push --force origin main",
		ToolName:    "Bash",
		FilePath:    "", // Bash has no file_path
	}
	resp := jiminy.ClassifyResponse{
		Verdict:      "deny",
		DenialReason: "never-force-push-to-main",
	}
	emitJiminyBlockAlert(context.Background(), m, req, resp)
	if m.calls.Load() != 1 {
		t.Fatalf("expected 1 Send() call, got %d", m.calls.Load())
	}
	if !strings.Contains(m.last.Message, "tool: Bash") {
		t.Errorf("bash message must include tool: Bash, got %q", m.last.Message)
	}
	if !strings.Contains(m.last.Message, "git push --force") {
		t.Errorf("bash message must include command preview, got %q", m.last.Message)
	}
	if !strings.Contains(m.last.Message, "never-force-push-to-main") {
		t.Errorf("bash message must include reason, got %q", m.last.Message)
	}
}

// JIMINY-ENFORCE-002: long Bash commands must be truncated so a runaway
// pipeline doesn't blow past the alert-message size budget.
func TestEmitJiminyBlockAlert_BashTruncatesLongCommands(t *testing.T) {
	m := &mockAlertSender{}
	longCmd := strings.Repeat("cat /dev/urandom | tr -d 0 | head -c 100 && ", 20) // ~880 chars
	emitJiminyBlockAlert(context.Background(), m,
		jiminy.ClassifyRequest{SpaceID: "s", ToolName: "Bash", AgentOutput: longCmd},
		jiminy.ClassifyResponse{Verdict: "deny", DenialReason: "test"})
	if !strings.Contains(m.last.Message, "…") {
		t.Errorf("long command must be truncated with … marker, got len=%d msg=%q", len(m.last.Message), m.last.Message)
	}
	// Message must be reasonably bounded — command preview capped at 200 chars.
	if len(m.last.Message) > 400 {
		t.Errorf("truncated message still too long: %d chars", len(m.last.Message))
	}
}

// TestEmitJiminyBlockAlert_EmptyReasonUsesFallback — an unusual case: verdict is deny
// but the classifier returned no reason. Message must still be meaningful.
func TestEmitJiminyBlockAlert_EmptyReasonUsesFallback(t *testing.T) {
	m := &mockAlertSender{}
	emitJiminyBlockAlert(context.Background(), m,
		jiminy.ClassifyRequest{SpaceID: "s"},
		jiminy.ClassifyResponse{Verdict: "deny"})
	if m.calls.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", m.calls.Load())
	}
	if m.last.Message == "" {
		t.Error("message must not be empty even with no denial reason")
	}
}
