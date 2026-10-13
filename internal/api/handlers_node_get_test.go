package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// J17-BRIEFING-PACKET-ENDPOINT-001 (2026-10-02) — pin tests for
// GET /v1/memory/nodes/{node_id}?space_id=X (the provenance-resolution
// endpoint that makes J17 briefing packet v1.1's `src:NODE_ID` contract
// cheap-to-honor).
//
// These tests cover ONLY the handler's input-validation surface + method
// guard. Live Neo4j path (200/404) is verified by UATS + Tier-3 smoke.
// nil-driver branches are the equivalent of the sibling TestHandleNodeMeta
// tests at `handlers_node_meta_test.go` shape.

func TestHandleGetNode_RejectsNonGET(t *testing.T) {
	s := &Server{}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(method, "/v1/memory/nodes/abc123?space_id=mdemg-dev", nil)
		w := httptest.NewRecorder()
		s.handleGetNode(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("method=%s got %d, want 405", method, w.Code)
		}
	}
}

func TestHandleGetNode_RejectsEmptyNodeID(t *testing.T) {
	s := &Server{}
	// Path = exactly `/v1/memory/nodes/` → TrimPrefix leaves empty → 400
	req := httptest.NewRequest(http.MethodGet, "/v1/memory/nodes/?space_id=mdemg-dev", nil)
	w := httptest.NewRecorder()
	s.handleGetNode(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty node_id: got %d, want 400", w.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(w.Body).Decode(&body)
	if !strings.Contains(body["error"].(string), "node_id") {
		t.Errorf("error should mention node_id, got %q", body["error"])
	}
}

func TestHandleGetNode_RejectsSubpaths(t *testing.T) {
	// A path with an embedded `/` after node_id (e.g. an accidental sub-route)
	// MUST reject at validation. Prevents the handler from silently matching
	// a node_id containing a slash — Cypher would never find it, but better
	// to fail fast with 400 than issue a doomed query.
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/memory/nodes/abc/sub?space_id=mdemg-dev", nil)
	w := httptest.NewRecorder()
	s.handleGetNode(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("subpath got %d, want 400", w.Code)
	}
}

func TestHandleGetNode_RejectsMissingSpaceID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/memory/nodes/abc123", nil)
	w := httptest.NewRecorder()
	s.handleGetNode(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing space_id: got %d, want 400", w.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(w.Body).Decode(&body)
	if !strings.Contains(body["error"].(string), "space_id") {
		t.Errorf("error should mention space_id, got %q", body["error"])
	}
}

// TestHandleNodeOperation_GETRoutes verifies the routing dispatch — GET on
// the bare /v1/memory/nodes/{id} path routes to handleGetNode (not
// handleDeleteNode). Regression pin: a future refactor that drops the
// `r.Method == http.MethodGet` case would silently 405 every provenance
// lookup, which is the exact failure mode the J17 packet's provenance
// contract is designed to prevent.
func TestHandleNodeOperation_GETRoutes(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/memory/nodes/abc123?space_id=mdemg-dev", nil)
	w := httptest.NewRecorder()
	// Use recover for the nil-driver panic that will fire after validation
	// passes (same shape as TestHandleNodeOperation_Routing).
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Panic means dispatch worked + validation passed + we got
				// to the Neo4j session (nil driver). Expected.
				t.Logf("expected nil-driver panic after dispatch: %v", r)
			}
		}()
		s.handleNodeOperation(w, req)
	}()
	// If the response was already written, it MUST NOT be 405 — that would
	// mean the GET branch isn't firing.
	if w.Code == http.StatusMethodNotAllowed {
		t.Error("GET on /v1/memory/nodes/{id} returned 405 — dispatch regression")
	}
}
