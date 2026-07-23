// FT-RECURSIVE-003 E4: pre-swap canary — held-call replay.
//
// Before a candidate takes production traffic, replay a deterministic probe
// slice of the pinned eval corpus against BOTH the production endpoint and
// the candidate (side-port), and compare STRUCTURALLY — parse/shape/finish,
// never score-exact (a better model may answer differently; a broken one
// answers malformed). Any probe where the candidate fails a structural check
// that production passes is a divergence; divergence blocks promotion.
package ftloop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// CanaryConfig controls the held-call replay.
type CanaryConfig struct {
	Enabled     bool          // FT_LOOP_CANARY_ENABLED
	ProbesPath  string        // FT_LOOP_CANARY_PROBES — jsonl with {messages:[{role,content}...], meta:{task_name}}
	ProbeCount  int           // FT_LOOP_CANARY_PROBE_COUNT (default 8)
	ProdBaseURL string        // production OpenAI-compat base (e.g. http://127.0.0.1:8102/v1)
	CandBaseURL string        // candidate side-port base (e.g. http://127.0.0.1:18102/v1)
	Timeout     time.Duration // per-call timeout (default 120s)
	MaxTokens   int           // per-call cap (default 3000 — never lower; truncation is catastrophic)
}

// CanaryProbe is one replayable prompt.
type CanaryProbe struct {
	TaskName string
	Messages []map[string]string
}

// CanaryResult summarizes a replay.
type CanaryResult struct {
	Probes      int
	Divergences []string // human-readable, one per diverging probe
}

// Pass reports whether the canary allows promotion.
func (r CanaryResult) Pass() bool { return len(r.Divergences) == 0 }

// LoadCanaryProbes reads the probe slice: the FIRST probe per task_name in
// file order, up to count — deterministic (no randomness; resume-safe) and
// task-diverse (one structural check per task family beats N of one task).
func LoadCanaryProbes(path string, count int) ([]CanaryProbe, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("canary probes: %w", err)
	}
	defer f.Close()

	if count <= 0 {
		count = 8
	}
	seen := map[string]bool{}
	var probes []CanaryProbe
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for sc.Scan() && len(probes) < count {
		var row struct {
			Messages []map[string]string `json:"messages"`
			Meta     struct {
				TaskName string `json:"task_name"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			continue
		}
		task := row.Meta.TaskName
		if task == "" || seen[task] || len(row.Messages) == 0 {
			continue
		}
		// Replay the prompt only (drop the recorded assistant turn).
		var msgs []map[string]string
		for _, m := range row.Messages {
			if m["role"] == "assistant" {
				break
			}
			msgs = append(msgs, m)
		}
		if len(msgs) == 0 {
			continue
		}
		seen[task] = true
		probes = append(probes, CanaryProbe{TaskName: task, Messages: msgs})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(probes) == 0 {
		return nil, fmt.Errorf("canary probes: no usable rows in %s", path)
	}
	return probes, nil
}

// probeOutcome is the structural fingerprint of one completion.
type probeOutcome struct {
	err          error
	finishReason string
	content      string
	jsonParses   bool
}

func completeOnce(ctx context.Context, baseURL string, p CanaryProbe, maxTokens int, timeout time.Duration) probeOutcome {
	body, _ := json.Marshal(map[string]any{
		"model":       "canary-probe",
		"messages":    p.Messages,
		"max_tokens":  maxTokens,
		"temperature": 0.0,
	})
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return probeOutcome{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return probeOutcome{err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return probeOutcome{err: fmt.Errorf("http %d", resp.StatusCode)}
	}
	var out struct {
		Choices []struct {
			Message      struct{ Content string } `json:"message"`
			FinishReason string                   `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Choices) == 0 {
		return probeOutcome{err: fmt.Errorf("malformed completion response: %v", err)}
	}
	content := out.Choices[0].Message.Content
	var js any
	jsonOK := json.Unmarshal([]byte(strings.TrimSpace(content)), &js) == nil
	return probeOutcome{
		finishReason: out.Choices[0].FinishReason,
		content:      content,
		jsonParses:   jsonOK,
	}
}

// RunCanary replays the probe slice against production and the candidate.
// Structural divergence rules (candidate judged ONLY where production is
// healthy on the same probe — a probe production also fails is skipped, not
// held against the candidate):
//  1. production ok, candidate errored
//  2. production finished (stop), candidate truncated (length)
//  3. production content non-empty, candidate empty
//  4. production output parses as JSON, candidate's does not
func RunCanary(ctx context.Context, cfg CanaryConfig) (CanaryResult, error) {
	probes, err := LoadCanaryProbes(cfg.ProbesPath, cfg.ProbeCount)
	if err != nil {
		return CanaryResult{}, err
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	maxTokens := cfg.MaxTokens
	if maxTokens < 3000 {
		maxTokens = 3000
	}

	res := CanaryResult{Probes: len(probes)}
	for _, p := range probes {
		prod := completeOnce(ctx, cfg.ProdBaseURL, p, maxTokens, timeout)
		if prod.err != nil {
			slog.Warn("canary: production probe unhealthy — skipping probe", "task", p.TaskName, "error", prod.err)
			continue
		}
		cand := completeOnce(ctx, cfg.CandBaseURL, p, maxTokens, timeout)
		switch {
		case cand.err != nil:
			res.Divergences = append(res.Divergences, fmt.Sprintf("%s: candidate errored: %v", p.TaskName, cand.err))
		case prod.finishReason == "stop" && cand.finishReason == "length":
			res.Divergences = append(res.Divergences, fmt.Sprintf("%s: candidate truncated (finish=length; production finished)", p.TaskName))
		case strings.TrimSpace(prod.content) != "" && strings.TrimSpace(cand.content) == "":
			res.Divergences = append(res.Divergences, fmt.Sprintf("%s: candidate empty (production non-empty)", p.TaskName))
		case prod.jsonParses && !cand.jsonParses:
			res.Divergences = append(res.Divergences, fmt.Sprintf("%s: candidate output not valid JSON (production's is)", p.TaskName))
		}
	}
	return res, nil
}
