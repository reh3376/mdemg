package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- UI Embed Tests ---

func TestUIHandler_ServesIndexHTML(t *testing.T) {
	handler := http.StripPrefix("/ui/", uiHandler())
	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /ui/ status: got %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "MDEMG Dashboard") {
		t.Error("index.html missing 'MDEMG Dashboard' title")
	}
	if !strings.Contains(body, "main.js") {
		t.Error("index.html missing main.js script reference")
	}
	if !strings.Contains(body, "tab-btn") {
		t.Error("index.html missing tab buttons")
	}
	if !strings.Contains(body, "space-select") {
		t.Error("index.html missing space selector")
	}
}

func TestUIHandler_ServesJSModules(t *testing.T) {
	handler := http.StripPrefix("/ui/", uiHandler())

	jsFiles := []string{
		"/ui/main.js",
		"/ui/api.js",
		"/ui/state.js",
	}
	for _, path := range jsFiles {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("GET %s: got %d, want %d", path, w.Code, http.StatusOK)
			}
			if w.Body.Len() == 0 {
				t.Errorf("GET %s: empty body", path)
			}
		})
	}
}

func TestUIHandler_ServesSubdirectoryFiles(t *testing.T) {
	handler := http.StripPrefix("/ui/", uiHandler())

	paths := []string{
		"/ui/tabs/status.js",
		"/ui/tabs/memory.js",
		"/ui/tabs/learning.js",
		"/ui/tabs/config.js",
		"/ui/tabs/logs.js",
		"/ui/tabs/rsic.js",
		"/ui/utils/dom.js",
		"/ui/utils/formatting.js",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("GET %s: got %d, want %d", path, w.Code, http.StatusOK)
			}
			if w.Body.Len() == 0 {
				t.Errorf("GET %s: empty body", path)
			}
		})
	}
}

func TestUIHandler_404ForNonexistent(t *testing.T) {
	handler := http.StripPrefix("/ui/", uiHandler())
	req := httptest.NewRequest(http.MethodGet, "/ui/nonexistent.js", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /ui/nonexistent.js: got %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestUIEmbed_FsSubStripsPrefix(t *testing.T) {
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		t.Fatalf("fs.Sub failed: %v", err)
	}
	// index.html should be at root of sub FS
	f, err := sub.Open("index.html")
	if err != nil {
		t.Fatalf("cannot open index.html in sub FS: %v", err)
	}
	f.Close()

	// tabs/status.js should be accessible
	f, err = sub.Open("tabs/status.js")
	if err != nil {
		t.Fatalf("cannot open tabs/status.js in sub FS: %v", err)
	}
	f.Close()
}

func TestUIEmbed_ContainsAllExpectedFiles(t *testing.T) {
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		t.Fatalf("fs.Sub failed: %v", err)
	}

	expected := []string{
		"index.html",
		"main.js",
		"api.js",
		"state.js",
		"tabs/status.js",
		"tabs/memory.js",
		"tabs/learning.js",
		"tabs/config.js",
		"tabs/logs.js",
		"tabs/rsic.js",
		"utils/dom.js",
		"utils/formatting.js",
	}
	for _, path := range expected {
		t.Run(path, func(t *testing.T) {
			f, err := sub.Open(path)
			if err != nil {
				t.Fatalf("missing embedded file: %s", path)
			}
			f.Close()
		})
	}
}

// --- Admin Config Handler Tests ---

func TestHandleAdminConfig_MethodNotAllowed(t *testing.T) {
	srv := &Server{}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/v1/admin/config", nil)
			w := httptest.NewRecorder()
			srv.handleAdminConfig(w, req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestHandleAdminConfig_ReturnsJSON(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/config", nil)
	w := httptest.NewRecorder()
	srv.handleAdminConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := resp["config"]; !ok {
		t.Error("missing 'config' key in response")
	}
	configs, ok := resp["config"].([]any)
	if !ok {
		t.Fatal("config should be an array")
	}
	if len(configs) == 0 {
		t.Error("config array is empty — should have default values")
	}

	// Verify structure of first entry
	first, ok := configs[0].(map[string]any)
	if !ok {
		t.Fatal("config entry should be an object")
	}
	for _, field := range []string{"key", "value", "source", "masked"} {
		if _, ok := first[field]; !ok {
			t.Errorf("config entry missing field: %s", field)
		}
	}
}

func TestHandleAdminConfig_SourcesAreValid(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/config", nil)
	w := httptest.NewRecorder()
	srv.handleAdminConfig(w, req)

	var resp struct {
		Config []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
			Masked bool   `json:"masked"`
			Value  string `json:"value"`
		} `json:"config"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	validSources := map[string]bool{"env": true, "yaml": true, "default": true}
	for _, entry := range resp.Config {
		if !validSources[entry.Source] {
			t.Errorf("invalid source '%s' for key '%s'", entry.Source, entry.Key)
		}
		if entry.Masked && entry.Value != "" && entry.Value != "****" {
			t.Errorf("masked key '%s' has value '%s', expected '****' or empty", entry.Key, entry.Value)
		}
	}
}

// --- Admin Logs Handler Tests ---

func TestHandleAdminLogs_MethodNotAllowed(t *testing.T) {
	srv := &Server{}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/v1/admin/logs", nil)
			w := httptest.NewRecorder()
			srv.handleAdminLogs(w, req)
			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestHandleAdminLogs_NoBuffer(t *testing.T) {
	srv := &Server{} // logBuffer is nil
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/logs", nil)
	w := httptest.NewRecorder()
	srv.handleAdminLogs(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleAdminLogs_WithBuffer(t *testing.T) {
	buf := NewLogRingBuffer(100)
	buf.Write([]byte("time=2026-03-30T12:00:00Z level=INFO msg=\"test entry\"\n"))

	srv := &Server{logBuffer: buf}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/logs", nil)
	w := httptest.NewRecorder()
	srv.handleAdminLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Entries []LogEntry `json:"entries"`
		Count   int        `json:"count"`
		Seq     int64      `json:"seq"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Count != 1 {
		t.Errorf("count: got %d, want 1", resp.Count)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(resp.Entries))
	}
	if resp.Entries[0].Level != "INFO" {
		t.Errorf("level: got %q, want INFO", resp.Entries[0].Level)
	}
	if resp.Entries[0].Message != "test entry" {
		t.Errorf("message: got %q, want 'test entry'", resp.Entries[0].Message)
	}
}

func TestHandleAdminLogs_LimitParam(t *testing.T) {
	buf := NewLogRingBuffer(100)
	for i := 0; i < 10; i++ {
		buf.Write([]byte("time=2026-03-30T12:00:00Z level=INFO msg=\"entry\"\n"))
	}

	srv := &Server{logBuffer: buf}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/logs?limit=3", nil)
	w := httptest.NewRecorder()
	srv.handleAdminLogs(w, req)

	var resp struct {
		Entries []LogEntry `json:"entries"`
		Count   int        `json:"count"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 3 {
		t.Errorf("count with limit=3: got %d, want 3", resp.Count)
	}
}

func TestHandleAdminLogs_LimitCappedAt500(t *testing.T) {
	buf := NewLogRingBuffer(100)
	srv := &Server{logBuffer: buf}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/logs?limit=9999", nil)
	w := httptest.NewRecorder()
	srv.handleAdminLogs(w, req)

	// Should not crash; limit is capped to 500
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleAdminLogs_InvalidLimit(t *testing.T) {
	buf := NewLogRingBuffer(100)
	srv := &Server{logBuffer: buf}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/logs?limit=abc", nil)
	w := httptest.NewRecorder()
	srv.handleAdminLogs(w, req)

	// Should fall back to default limit=200 and not error
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
}
