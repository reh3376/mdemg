package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mdemg/internal/models"
)

func TestBuildIngestArgsFromConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     map[string]any
		listenAddr string
		wantArgs   map[string]bool // args that must be present
		wantAbsent []string        // args that must NOT be present
	}{
		{
			name: "basic config",
			config: map[string]any{
				"space_id": "test-space",
				"path":     "/tmp/code",
			},
			listenAddr: ":9999",
			wantArgs: map[string]bool{
				"--path":          true,
				"/tmp/code":       true,
				"--space-id":      true,
				"test-space":      true,
				"--progress-json": true,
			},
		},
		{
			name: "full config",
			config: map[string]any{
				"space_id":        "my-space",
				"path":            "/home/user/project",
				"batch_size":      200,
				"workers":         8,
				"timeout_seconds": 600,
				"extract_symbols": true,
				"consolidate":     false,
				"include_tests":   true,
				"incremental":     true,
				"since_commit":    "abc123",
				"exclude_dirs":    []string{"vendor", "node_modules"},
				"limit":           1000,
			},
			listenAddr: "http://localhost:8080",
			wantArgs: map[string]bool{
				"--batch=200":            true,
				"--workers=8":            true,
				"--timeout=600":          true,
				"--extract-symbols=true": true,
				"--consolidate=false":    true,
				"--include-tests":        true,
				"--incremental":          true,
				"--progress-json":        true,
				"--limit=1000":           true,
			},
		},
		{
			name: "exclude_dirs as []any",
			config: map[string]any{
				"space_id":     "test",
				"path":         "/tmp",
				"exclude_dirs": []any{"vendor", "dist"},
			},
			listenAddr: ":9999",
			wantArgs: map[string]bool{
				"--exclude":       true,
				"vendor,dist":     true,
				"--progress-json": true,
			},
		},
		{
			name: "empty listenAddr defaults",
			config: map[string]any{
				"space_id": "test",
				"path":     "/tmp",
			},
			listenAddr: "",
			wantArgs: map[string]bool{
				"http://localhost:9999": true,
			},
		},
		{
			name: "dry_run flag",
			config: map[string]any{
				"space_id": "test",
				"path":     "/tmp",
				"dry_run":  true,
			},
			listenAddr: ":9999",
			wantArgs: map[string]bool{
				"--dry-run": true,
			},
		},
		{
			// INGEST-TRIGGER-FORWARD-001: language toggles + archive_deleted
			// emit explicit =true/=false flags when present in config.
			name: "language toggles forwarded",
			config: map[string]any{
				"space_id":        "test",
				"path":            "/tmp",
				"include_md":      false,
				"include_ts":      true,
				"include_py":      false,
				"archive_deleted": false,
			},
			listenAddr: ":9999",
			wantArgs: map[string]bool{
				"--include-md=false":      true,
				"--include-ts=true":       true,
				"--include-py=false":      true,
				"--archive-deleted=false": true,
			},
		},
		{
			// Omitted toggles emit NO flag — the CLI default stays authoritative.
			name: "language toggles omitted when absent",
			config: map[string]any{
				"space_id": "test",
				"path":     "/tmp",
			},
			listenAddr: ":9999",
			wantArgs: map[string]bool{
				"--progress-json": true,
			},
			wantAbsent: []string{
				"--include-md=true", "--include-md=false",
				"--include-ts=true", "--include-ts=false",
				"--include-py=true", "--include-py=false",
				"--archive-deleted=true", "--archive-deleted=false",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildIngestArgsFromConfig(tt.config, tt.listenAddr)
			argStr := strings.Join(args, " ")

			for wantArg := range tt.wantArgs {
				found := false
				for _, a := range args {
					if a == wantArg {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected arg %q in args: %s", wantArg, argStr)
				}
			}

			for _, absent := range tt.wantAbsent {
				for _, a := range args {
					if a == absent {
						t.Errorf("unexpected arg %q in args: %s", absent, argStr)
					}
				}
			}
		})
	}
}

// INGEST-TRIGGER-FORWARD-001: request pointer-bools land in the job config
// only when set; omitted fields stay out of the config entirely.
func TestBuildIngestConfigFromRequest_PointerBools(t *testing.T) {
	f := false
	tr := true

	set := buildIngestConfigFromRequest(models.IngestTriggerRequest{
		SpaceID:         "s",
		Path:            "/tmp",
		IncludeMarkdown: &f,
		IncludeTS:       &tr,
		IncludePython:   &f,
		ArchiveDeleted:  &f,
	})
	for k, want := range map[string]bool{
		"include_md": false, "include_ts": true, "include_py": false, "archive_deleted": false,
	} {
		got, ok := set[k].(bool)
		if !ok {
			t.Errorf("%s: expected bool in config, missing", k)
			continue
		}
		if got != want {
			t.Errorf("%s: got %v, want %v", k, got, want)
		}
	}

	unset := buildIngestConfigFromRequest(models.IngestTriggerRequest{SpaceID: "s", Path: "/tmp"})
	for _, k := range []string{"include_md", "include_ts", "include_py", "archive_deleted"} {
		if _, present := unset[k]; present {
			t.Errorf("%s: expected absent from config when request field is nil", k)
		}
	}
}

func TestParseProgressEvent(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    ingestProgressEvent
		wantErr bool
	}{
		{
			name: "discovery_complete",
			line: `{"event":"discovery_complete","total":4522}`,
			want: ingestProgressEvent{Event: "discovery_complete", Total: 4522},
		},
		{
			name: "batch_progress",
			line: `{"event":"batch_progress","current":100,"total":4522,"rate":15.2}`,
			want: ingestProgressEvent{Event: "batch_progress", Current: 100, Total: 4522, Rate: 15.2},
		},
		{
			name: "consolidation_start",
			line: `{"event":"consolidation_start"}`,
			want: ingestProgressEvent{Event: "consolidation_start"},
		},
		{
			name: "complete",
			line: `{"event":"complete","total":4522,"ingested":4520,"errors":2,"duration":"5m6s"}`,
			want: ingestProgressEvent{Event: "complete", Total: 4522, Ingested: 4520, Errors: 2, Duration: "5m6s"},
		},
		{
			name:    "invalid JSON",
			line:    `not json at all`,
			wantErr: true,
		},
		{
			name:    "empty line",
			line:    ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var evt ingestProgressEvent
			err := json.Unmarshal([]byte(tt.line), &evt)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if evt.Event != tt.want.Event {
				t.Errorf("event: got %q, want %q", evt.Event, tt.want.Event)
			}
			if evt.Total != tt.want.Total {
				t.Errorf("total: got %d, want %d", evt.Total, tt.want.Total)
			}
			if evt.Current != tt.want.Current {
				t.Errorf("current: got %d, want %d", evt.Current, tt.want.Current)
			}
			if evt.Ingested != tt.want.Ingested {
				t.Errorf("ingested: got %d, want %d", evt.Ingested, tt.want.Ingested)
			}
			if evt.Errors != tt.want.Errors {
				t.Errorf("errors: got %d, want %d", evt.Errors, tt.want.Errors)
			}
			if evt.Duration != tt.want.Duration {
				t.Errorf("duration: got %q, want %q", evt.Duration, tt.want.Duration)
			}
		})
	}
}

func TestHandleIngestFiles_Validation(t *testing.T) {
	// Create a minimal server (no real services needed for validation tests)
	srv := &Server{}

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "missing space_id",
			body:       `{"files":["/tmp/test.go"]}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "space_id",
		},
		{
			name:       "empty files array",
			body:       `{"space_id":"test","files":[]}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "files",
		},
		{
			name:       "missing files field",
			body:       `{"space_id":"test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "files",
		},
		{
			name:       "nonexistent files",
			body:       `{"space_id":"test","files":["/nonexistent/a.go","/nonexistent/b.go"]}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "missing_files",
		},
		{
			name:       "invalid JSON",
			body:       `{invalid`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/memory/ingest/files", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			srv.handleIngestFiles(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d (body: %s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantError != "" && !strings.Contains(w.Body.String(), tt.wantError) {
				t.Errorf("expected error containing %q, got: %s", tt.wantError, w.Body.String())
			}
		})
	}
}

func TestBuildIngestArgsFromConfig_QuietFlag(t *testing.T) {
	// When quiet is set in config, --quiet should be passed
	config := map[string]any{
		"space_id": "test-space",
		"path":     "/tmp/code",
		"quiet":    true,
	}

	args := buildIngestArgsFromConfig(config, ":9999")
	argStr := strings.Join(args, " ")

	found := false
	for _, a := range args {
		if a == "--quiet" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --quiet in args: %s", argStr)
	}
}

func TestHandleIngestFiles_MethodNotAllowed(t *testing.T) {
	srv := &Server{}

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/v1/memory/ingest/files", nil)
			w := httptest.NewRecorder()

			srv.handleIngestFiles(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("status: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}
