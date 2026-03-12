package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewResultClient(t *testing.T) {
	rc := NewResultClient("http://localhost:9999", "test-space")
	if rc.endpoint != "http://localhost:9999" {
		t.Errorf("endpoint = %s, want http://localhost:9999", rc.endpoint)
	}
	if rc.spaceID != "test-space" {
		t.Errorf("spaceID = %s, want test-space", rc.spaceID)
	}
	if rc.client == nil {
		t.Error("client should not be nil")
	}
}

func TestStoreResult_Success(t *testing.T) {
	var received observeRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/conversation/observe" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("unexpected Content-Type: %s", ct)
		}

		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	rc := NewResultClient(srv.URL, "test-space")
	err := rc.StoreResult("test drift content", "error")
	if err != nil {
		t.Fatalf("StoreResult: %v", err)
	}

	if received.SpaceID != "test-space" {
		t.Errorf("space_id = %s, want test-space", received.SpaceID)
	}
	if received.SessionID != "uxts-module" {
		t.Errorf("session_id = %s, want uxts-module", received.SessionID)
	}
	if received.Content != "test drift content" {
		t.Errorf("content = %s, want 'test drift content'", received.Content)
	}
	if received.ObsType != "error" {
		t.Errorf("obs_type = %s, want error", received.ObsType)
	}
}

func TestStoreResult_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rc := NewResultClient(srv.URL, "test-space")
	err := rc.StoreResult("test", "error")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestStoreResult_Unreachable(t *testing.T) {
	rc := NewResultClient("http://127.0.0.1:1", "test-space")
	err := rc.StoreResult("test", "error")
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestStoreDriftResults_NoDrift(t *testing.T) {
	// Should not make any HTTP calls when no drifted specs
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rc := NewResultClient(srv.URL, "test-space")
	rc.StoreDriftResults("uats", nil, 10)

	if callCount != 0 {
		t.Errorf("expected 0 HTTP calls for no drift, got %d", callCount)
	}
}

func TestStoreDriftResults_WithDrift(t *testing.T) {
	var received observeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rc := NewResultClient(srv.URL, "test-space")
	drifted := []string{"spec1.uats.json", "spec2.uats.json"}
	rc.StoreDriftResults("uats", drifted, 8)

	if received.ObsType != "error" {
		t.Errorf("obs_type = %s, want error", received.ObsType)
	}
	if received.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestJoinMax(t *testing.T) {
	tests := []struct {
		items []string
		max   int
		want  string
	}{
		{[]string{"a", "b"}, 5, "[a b]"},
		{[]string{"a", "b", "c", "d", "e", "f"}, 3, "[a b c] and 3 more"},
	}
	for _, tt := range tests {
		got := joinMax(tt.items, tt.max)
		if got != tt.want {
			t.Errorf("joinMax(%v, %d) = %q, want %q", tt.items, tt.max, got, tt.want)
		}
	}
}
