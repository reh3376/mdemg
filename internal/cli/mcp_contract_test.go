package cli

// MCP tool contract suite (MCP-REVIVE-001). The 1,635-line MCP surface had
// ZERO tests. These are UATS-style contracts at the Go level: each tool
// handler is invoked with constructed arguments against an httptest MDEMG
// backend, and the test asserts the HTTP mapping (method, path, body —
// especially the space-resolution chain: explicit space_id param >
// MDEMG_SPACE_ID-derived server default > ide-agent) plus response shaping.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// capturedCall records one HTTP request the handler made.
type capturedCall struct {
	Method string
	Path   string
	Query  map[string]string
	Body   map[string]any
}

type mcpTestBackend struct {
	mu       sync.Mutex
	calls    []capturedCall
	response map[string]any // served for every request
	status   int
}

func (b *mcpTestBackend) handler(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cc := capturedCall{Method: r.Method, Path: r.URL.Path, Query: map[string]string{}}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			cc.Query[k] = v[0]
		}
	}
	if r.Body != nil {
		data, _ := io.ReadAll(r.Body)
		if len(data) > 0 {
			_ = json.Unmarshal(data, &cc.Body)
		}
	}
	b.calls = append(b.calls, cc)
	w.Header().Set("Content-Type", "application/json")
	status := b.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	resp := b.response
	if resp == nil {
		resp = map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (b *mcpTestBackend) lastCall(t *testing.T) capturedCall {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.calls) == 0 {
		t.Fatal("handler made no HTTP calls")
	}
	return b.calls[len(b.calls)-1]
}

func newTestMCPServer(t *testing.T, backend *mcpTestBackend, defaultSpace string) *mcpServer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(backend.handler))
	t.Cleanup(srv.Close)
	if defaultSpace == "" {
		defaultSpace = defaultSpaceID
	}
	return &mcpServer{
		endpoint:     srv.URL,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		defaultSpace: defaultSpace,
	}
}

func toolRequest(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil {
		t.Fatal("nil tool result")
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := mcp.AsTextContent(c); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

// --- Space resolution chain (the sprint's core contract) ---

func TestSpaceResolution_Precedence(t *testing.T) {
	m := &mcpServer{defaultSpace: "env-space"}
	if got := m.resolveSpace(map[string]any{"space_id": "explicit"}); got != "explicit" {
		t.Errorf("explicit param lost: %q", got)
	}
	if got := m.resolveSpace(map[string]any{}); got != "env-space" {
		t.Errorf("env default lost: %q", got)
	}
	bare := &mcpServer{defaultSpace: defaultSpaceID}
	if got := bare.resolveSpace(map[string]any{}); got != "ide-agent" {
		t.Errorf("back-compat fallback lost: %q", got)
	}
}

// --- memory_store ---

func TestMemoryStore_Contract(t *testing.T) {
	b := &mcpTestBackend{response: map[string]any{"node_id": "n_test", "embedding_dims": float64(4096)}}
	m := newTestMCPServer(t, b, "env-space")

	res, err := m.memoryStoreHandler(context.Background(), toolRequest(map[string]any{
		"content": "test observation", "tags": "a, b", "source": "unit-test",
	}))
	if err != nil {
		t.Fatal(err)
	}
	call := b.lastCall(t)
	if call.Path != "/v1/memory/ingest" || call.Method != http.MethodPost {
		t.Errorf("call = %s %s", call.Method, call.Path)
	}
	if call.Body["space_id"] != "env-space" {
		t.Errorf("space_id = %v, want env default", call.Body["space_id"])
	}
	if call.Body["content"] != "test observation" {
		t.Errorf("content = %v", call.Body["content"])
	}
	if tags, _ := call.Body["tags"].([]any); len(tags) != 2 {
		t.Errorf("tags = %v", call.Body["tags"])
	}
	if !strings.Contains(resultText(t, res), "n_test") {
		t.Errorf("result missing node id: %q", resultText(t, res))
	}

	// explicit space wins
	_, err = m.memoryStoreHandler(context.Background(), toolRequest(map[string]any{
		"content": "x", "space_id": "other-space",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := b.lastCall(t).Body["space_id"]; got != "other-space" {
		t.Errorf("explicit space_id = %v", got)
	}

	// missing content → tool error, no HTTP call
	before := len(b.calls)
	res, _ = m.memoryStoreHandler(context.Background(), toolRequest(map[string]any{}))
	if !res.IsError {
		t.Error("missing content should be a tool error")
	}
	if len(b.calls) != before {
		t.Error("validation failure must not reach the backend")
	}
}

// --- memory_recall ---

func TestMemoryRecall_Contract(t *testing.T) {
	b := &mcpTestBackend{response: map[string]any{"results": []any{}}}
	m := newTestMCPServer(t, b, "env-space")

	_, err := m.memoryRecallHandler(context.Background(), toolRequest(map[string]any{
		"query": "how does retrieval work", "limit": float64(5),
	}))
	if err != nil {
		t.Fatal(err)
	}
	call := b.lastCall(t)
	if call.Path != "/v1/memory/retrieve" {
		t.Errorf("path = %s", call.Path)
	}
	if call.Body["space_id"] != "env-space" || call.Body["query_text"] != "how does retrieval work" {
		t.Errorf("body = %v", call.Body)
	}
	if call.Body["top_k"] != float64(5) {
		t.Errorf("top_k = %v", call.Body["top_k"])
	}

	_, _ = m.memoryRecallHandler(context.Background(), toolRequest(map[string]any{
		"query": "q", "space_id": "explicit-space",
	}))
	if got := b.lastCall(t).Body["space_id"]; got != "explicit-space" {
		t.Errorf("explicit space_id = %v", got)
	}
}

// --- memory_associate (both lookups + the ingest share one space) ---

func TestMemoryAssociate_Contract(t *testing.T) {
	b := &mcpTestBackend{response: map[string]any{
		"results": []any{map[string]any{"node_id": "n1", "name": "a", "content": "c"}},
	}}
	m := newTestMCPServer(t, b, "env-space")

	_, err := m.memoryAssociateHandler(context.Background(), toolRequest(map[string]any{
		"source_query": "src", "target_query": "dst", "space_id": "explicit-space",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.calls) < 3 {
		t.Fatalf("expected 3 calls (2 lookups + ingest), got %d", len(b.calls))
	}
	for i, c := range b.calls {
		if c.Body["space_id"] != "explicit-space" {
			t.Errorf("call %d space_id = %v — association must stay in ONE space", i, c.Body["space_id"])
		}
	}
}

// --- memory_reflect ---

func TestMemoryReflect_Contract(t *testing.T) {
	b := &mcpTestBackend{response: map[string]any{"results": []any{}}}
	m := newTestMCPServer(t, b, "env-space")
	_, err := m.memoryReflectHandler(context.Background(), toolRequest(map[string]any{
		"topic": "retrieval pipeline",
	}))
	if err != nil {
		t.Fatal(err)
	}
	call := b.lastCall(t)
	if call.Path != "/v1/memory/retrieve" || call.Body["space_id"] != "env-space" {
		t.Errorf("call = %s body=%v", call.Path, call.Body)
	}
}

// --- memory_symbols (query params, not body) ---

func TestMemorySymbols_Contract(t *testing.T) {
	b := &mcpTestBackend{response: map[string]any{"symbols": []any{}}}
	m := newTestMCPServer(t, b, "env-space")
	_, err := m.memorySymbolsHandler(context.Background(), toolRequest(map[string]any{
		"name": "Retrieve", "space_id": "explicit-space",
	}))
	if err != nil {
		t.Fatal(err)
	}
	call := b.lastCall(t)
	if !strings.HasPrefix(call.Path, "/v1/memory/symbols") {
		t.Errorf("path = %s", call.Path)
	}
	if call.Query["space_id"] != "explicit-space" {
		t.Errorf("query space_id = %v", call.Query["space_id"])
	}
}

// --- memory_ingest_trigger ---

func TestMemoryIngestTrigger_Contract(t *testing.T) {
	b := &mcpTestBackend{response: map[string]any{"job_id": "j1", "status": "queued"}}
	m := newTestMCPServer(t, b, "env-space")
	_, err := m.memoryIngestTriggerHandler(context.Background(), toolRequest(map[string]any{
		"path": "/tmp/repo",
	}))
	if err != nil {
		t.Fatal(err)
	}
	call := b.lastCall(t)
	if call.Body["space_id"] != "env-space" || call.Body["path"] != "/tmp/repo" {
		t.Errorf("body = %v", call.Body)
	}
}

// --- backend error surfaces as tool error, not Go error ---

func TestBackendError_SurfacesAsToolError(t *testing.T) {
	b := &mcpTestBackend{status: http.StatusInternalServerError, response: map[string]any{"error": "boom"}}
	m := newTestMCPServer(t, b, "")
	res, err := m.memoryRecallHandler(context.Background(), toolRequest(map[string]any{"query": "q"}))
	if err != nil {
		t.Fatalf("handler returned Go error (must be tool error): %v", err)
	}
	if !res.IsError {
		t.Error("backend 500 must surface as a tool error result")
	}
}
