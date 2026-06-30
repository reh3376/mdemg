// The recursive-retrain pipeline stages (FT-RECURSIVE-002 Phase 6b, Epic 6).
//
// Each stage builds + runs the exact command validated live in Epic 6 (see
// docs/development/ft-recursive-002/epic6_issues.md) and threads its artifact to
// the next:  export → curate → train → convert → gate.
package ftloop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// workRoot is the artifact root for cycles.
func (c *Controller) workRoot() string {
	if c.cfg.WorkDir != "" {
		return c.cfg.WorkDir
	}
	return filepath.Join(os.TempDir(), "mdemg-ft-loop")
}

// resolvePython prefers the neural venv interpreter (where mlx_lm lives) when
// PythonBin is the bare default (E6-1); an explicit non-default wins.
func (c *Controller) resolvePython() string {
	if c.cfg.PythonBin != "" && c.cfg.PythonBin != "python3" {
		return c.cfg.PythonBin
	}
	venv := filepath.Join(c.cfg.RepoDir, "neural", ".venv", "bin", "python")
	if _, err := os.Stat(venv); err == nil {
		return venv
	}
	if c.cfg.PythonBin != "" {
		return c.cfg.PythonBin
	}
	return "python3"
}

// execCmd runs bin+args in dir under ctx (a shutdown cancels long stages) and
// returns combined output. Production impl behind the overridable runCmd.
func (c *Controller) execCmd(ctx context.Context, label, dir, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // G204: controller-constructed args, not user input
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w (output tail: %s)", label, err, tail(out, 500))
	}
	return string(out), nil
}

func (c *Controller) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.cfg.RepoDir, p)
}

// stageExport materializes the curate input: `mdemg data export` the task's
// llm_interactions, extract llm_interactions.jsonl into <work>/input/ (E6-5).
func (c *Controller) stageExport(ctx context.Context, work string) (string, error) {
	inputDir := filepath.Join(work, "input")
	if err := os.MkdirAll(inputDir, 0o750); err != nil {
		return "", err
	}
	archive := filepath.Join(work, "export.tar.gz")
	since := c.now().Add(-time.Duration(c.cfg.ExportSinceDays) * 24 * time.Hour).UTC().Format(time.RFC3339)
	space := c.cfg.SpaceID
	if space == "" {
		space = "mdemg-dev"
	}
	bin := c.cfg.MdemgBin
	if bin == "" {
		bin = "mdemg"
	}
	if _, err := c.runCmd(ctx, "export", c.cfg.RepoDir, bin,
		[]string{"data", "export", "--space-id", space, "--tables", "llm_interactions",
			"--since", since, "--output", archive}); err != nil {
		return "", err
	}
	// Extract the archive and locate llm_interactions.jsonl.
	if _, err := c.runCmd(ctx, "export-extract", work, "tar", []string{"-xzf", archive, "-C", work}); err != nil {
		return "", err
	}
	src, err := findFile(work, "llm_interactions.jsonl")
	if err != nil {
		return "", fmt.Errorf("export: %w", err)
	}
	if err := copyFile(src, filepath.Join(inputDir, "llm_interactions.jsonl")); err != nil {
		return "", err
	}
	return inputDir, nil
}

// stageCurate runs paradigm_router (venv python, cwd neural/) → versioned SFT
// dir, then bridges the val.jsonl→valid.jsonl naming train expects (E6-3/E6-10).
func (c *Controller) stageCurate(ctx context.Context, cycleID, work, inputDir string) (string, error) {
	curated := filepath.Join(work, "curated")
	if _, err := c.runCmd(ctx, "curate", filepath.Join(c.cfg.RepoDir, "neural"), c.resolvePython(),
		[]string{"-m", "training.paradigm_router",
			"--spec", c.abs(c.cfg.UaitsSpec),
			"--input-dir", inputDir,
			"--output-dir", curated,
			"--version", cycleID}); err != nil {
		return "", err
	}
	versioned := filepath.Join(curated, "sft_interactions", "versioned")
	// E6-10: mlx_lm.lora expects valid.jsonl; paradigm_router emits val.jsonl.
	val := filepath.Join(versioned, "val.jsonl")
	if _, err := os.Stat(val); err == nil {
		if err := copyFile(val, filepath.Join(versioned, "valid.jsonl")); err != nil {
			return "", fmt.Errorf("curate val→valid: %w", err)
		}
	}
	if _, err := os.Stat(filepath.Join(versioned, "train.jsonl")); err != nil {
		return "", fmt.Errorf("curate produced no train.jsonl at %s", versioned)
	}
	return versioned, nil
}

// stageTrain runs train_ft (venv python, cwd neural/) on the curated dataset →
// a LoRA adapter dir (E6-3). The base SHA pin (E6-8) is passed explicitly.
func (c *Controller) stageTrain(ctx context.Context, work, versionedDir string) (string, error) {
	adapterDir := filepath.Join(work, "adapter")
	if err := os.MkdirAll(adapterDir, 0o750); err != nil {
		return "", err
	}
	if _, err := c.runCmd(ctx, "train", filepath.Join(c.cfg.RepoDir, "neural"), c.resolvePython(),
		[]string{"-m", "training.train_ft", "--tier", "1", "--mode", "sft",
			"--base-model", c.abs(c.cfg.BaseModel),
			"--expected-sha256", c.cfg.BaseSHA,
			"--dataset", versionedDir,
			"--adapter-path", adapterDir,
			"--rank", strconv.Itoa(c.cfg.LoraRank),
			"--alpha", strconv.Itoa(c.cfg.LoraAlpha),
			"--n-epochs", strconv.Itoa(c.cfg.EpochsCap)}); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(adapterDir, "adapters.safetensors")); err != nil {
		return "", fmt.Errorf("train produced no adapters.safetensors")
	}
	return adapterDir, nil
}

// stageConvert: fuse --dequantize (E6-14) → convert_hf_to_gguf f16 →
// llama-quantize Q5_K_M → the candidate GGUF (E6-4).
func (c *Controller) stageConvert(ctx context.Context, work, adapterDir string) (string, error) {
	fusedBF16 := filepath.Join(work, "fused-bf16")
	f16 := filepath.Join(work, "candidate-f16.gguf")
	candidate := filepath.Join(work, "candidate-Q5_K_M.gguf")
	py := c.resolvePython()

	if _, err := c.runCmd(ctx, "convert-fuse", c.cfg.RepoDir, py,
		[]string{"-m", "mlx_lm.fuse", "--model", c.abs(c.cfg.BaseModel),
			"--adapter-path", adapterDir, "--save-path", fusedBF16, "--dequantize"}); err != nil {
		return "", err
	}
	conv, err := exec.LookPath("convert_hf_to_gguf.py")
	if err != nil {
		conv = "convert_hf_to_gguf.py" // fall through; execCmd will surface a clear error
	}
	if _, err := c.runCmd(ctx, "convert-gguf", c.cfg.RepoDir, py,
		[]string{conv, fusedBF16, "--outtype", "f16", "--outfile", f16}); err != nil {
		return "", err
	}
	if _, err := c.runCmd(ctx, "convert-quantize", c.cfg.RepoDir, "llama-quantize",
		[]string{f16, candidate, "Q5_K_M"}); err != nil {
		return "", err
	}
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("convert produced no candidate GGUF")
	}
	return candidate, nil
}

// stageGate serves the candidate on a side-port, runs run_benchmark against it,
// and passes only if the aggregate score ≥ GateMinScore with real (non-zero)
// calls — the no-zero-call discipline from the run_record (E6-3).
func (c *Controller) stageGate(ctx context.Context, work, candidate string) error {
	port := c.cfg.GatePort
	report := filepath.Join(work, "gate-report.json")

	// Start a side-port llama-server for the candidate; stop it on return.
	srv := exec.CommandContext(ctx, "llama-server", "--model", candidate,
		"--port", strconv.Itoa(port), "--host", "127.0.0.1",
		"--ctx-size", "8192", "--parallel", "1", "--jinja") //nolint:gosec // G204: controller-constructed
	srv.Dir = c.cfg.RepoDir
	if err := srv.Start(); err != nil {
		return fmt.Errorf("gate: start candidate server: %w", err)
	}
	defer func() {
		if srv.Process != nil {
			_ = srv.Process.Kill()
			_, _ = srv.Process.Wait()
		}
	}()

	// Readiness = /health (NOT /v1/models — it answers before the model loads).
	if err := waitHealth(ctx, fmt.Sprintf("http://127.0.0.1:%d/health", port), 120*time.Second); err != nil {
		return fmt.Errorf("gate: candidate server not ready: %w", err)
	}

	args := []string{"-m", "neural.benchmarks.run_benchmark",
		"--config", c.abs(c.cfg.BenchmarkConfig),
		"--mlx-base-url", fmt.Sprintf("http://127.0.0.1:%d/v1", port),
		"--mlx-model-name", "ft-loop-candidate",
		"--out", report}
	if c.cfg.GateTaskFilter != "" {
		args = append(args, "--task-filter", c.cfg.GateTaskFilter)
	}
	if _, err := c.runCmd(ctx, "gate-benchmark", c.cfg.RepoDir, c.resolvePython(), args); err != nil {
		return err
	}

	score, truncated, err := readGateReport(report)
	if err != nil {
		return fmt.Errorf("gate: %w", err)
	}
	if truncated > 0 {
		return fmt.Errorf("gate FAIL: %d truncated rows (no-zero-call/truncation discipline)", truncated)
	}
	if score < c.cfg.GateMinScore {
		return fmt.Errorf("gate FAIL: aggregate %.4f < floor %.4f", score, c.cfg.GateMinScore)
	}
	return nil
}

// --- helpers ---

func waitHealth(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("health timeout after %s", timeout)
}

// readGateReport extracts the aggregate score + truncated-row count from a
// run_benchmark report, tolerating the common key spellings.
func readGateReport(path string) (score float64, truncated int, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	var d map[string]any
	if err := json.Unmarshal(b, &d); err != nil {
		return 0, 0, err
	}
	for _, k := range []string{"aggregate_weighted_score", "aggregate_score", "aggregate"} {
		if v, ok := d[k].(float64); ok {
			score = v
			break
		}
	}
	for _, k := range []string{"truncated_rows", "max_truncated_rows", "truncated"} {
		if v, ok := d[k].(float64); ok {
			truncated = int(v)
			break
		}
	}
	return score, truncated, nil
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o600)
}

func findFile(root, name string) (string, error) {
	var found string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && info.Name() == name {
			found = p
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("file %q not found under %s", name, root)
	}
	return found, nil
}
