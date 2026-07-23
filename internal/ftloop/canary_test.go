package ftloop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProbes(t *testing.T) string {
	t.Helper()
	rows := []string{
		`{"messages":[{"role":"system","content":"s"},{"role":"user","content":"u1"},{"role":"assistant","content":"a"}],"meta":{"task_name":"task.a"}}`,
		`{"messages":[{"role":"user","content":"u2-second-of-a"}],"meta":{"task_name":"task.a"}}`,
		`{"messages":[{"role":"user","content":"u3"}],"meta":{"task_name":"task.b"}}`,
	}
	p := filepath.Join(t.TempDir(), "probes.jsonl")
	f, _ := os.Create(p)
	for _, r := range rows {
		f.WriteString(r + "\n")
	}
	f.Close()
	return p
}

func TestLoadCanaryProbes_FirstPerTaskDeterministic(t *testing.T) {
	probes, err := LoadCanaryProbes(writeProbes(t), 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != 2 || probes[0].TaskName != "task.a" || probes[1].TaskName != "task.b" {
		t.Fatalf("probes: %+v", probes)
	}
	// assistant turn dropped
	for _, m := range probes[0].Messages {
		if m["role"] == "assistant" {
			t.Error("assistant turn must be dropped")
		}
	}
}

func fakeLLM(t *testing.T, content, finish string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != 200 {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]string{"content": content},
				"finish_reason": finish,
			}},
		})
	}))
}

func TestRunCanary_StructuralDivergences(t *testing.T) {
	probesPath := writeProbes(t)
	prod := fakeLLM(t, `{"ok": true}`, "stop", 200)
	defer prod.Close()

	cases := []struct {
		name          string
		cand          *httptest.Server
		wantDiverge   bool
		divergeSubstr string
	}{
		{"identical-structure", fakeLLM(t, `{"different": "answer"}`, "stop", 200), false, ""},
		{"candidate-errors", fakeLLM(t, "", "", 500), true, "errored"},
		{"candidate-truncates", fakeLLM(t, `{"partial`, "length", 200), true, "truncated"},
		{"candidate-not-json", fakeLLM(t, "plain text answer", "stop", 200), true, "not valid JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.cand.Close()
			res, err := RunCanary(context.Background(), CanaryConfig{
				ProbesPath: probesPath, ProbeCount: 2,
				ProdBaseURL: prod.URL, CandBaseURL: tc.cand.URL,
			})
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantDiverge == res.Pass() {
				t.Fatalf("pass=%v divergences=%v", res.Pass(), res.Divergences)
			}
			if tc.wantDiverge && !containsSubstr(res.Divergences, tc.divergeSubstr) {
				t.Errorf("want divergence containing %q, got %v", tc.divergeSubstr, res.Divergences)
			}
		})
	}
}

// Production failing a probe must NOT count against the candidate.
func TestRunCanary_SkipsWhenProductionUnhealthy(t *testing.T) {
	probesPath := writeProbes(t)
	prod := fakeLLM(t, "", "", 500)
	defer prod.Close()
	cand := fakeLLM(t, "anything", "stop", 200)
	defer cand.Close()
	res, err := RunCanary(context.Background(), CanaryConfig{
		ProbesPath: probesPath, ProbeCount: 2,
		ProdBaseURL: prod.URL, CandBaseURL: cand.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pass() {
		t.Errorf("expected pass when production itself is unhealthy: %v", res.Divergences)
	}
}

func containsSubstr(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
