package ftloop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestController_DisabledRunReturnsNil(t *testing.T) {
	c := NewController(nil, nil, nil, ControllerConfig{Enabled: false})
	if err := c.Run(context.Background()); err != nil {
		t.Errorf("disabled controller Run should return nil, got %v", err)
	}
}

// captureRunner records each runCmd call and creates the artifact the next
// stage's existence-check expects, so the pipeline command-construction can be
// validated end-to-end without real subprocesses.
type capturedCmd struct {
	label, dir, bin string
	args            []string
}

func newCaptureController(t *testing.T, cfg ControllerConfig) (*Controller, *[]capturedCmd) {
	t.Helper()
	var calls []capturedCmd
	c := NewController(nil, nil, nil, cfg)
	c.runCmd = func(_ context.Context, label, dir, bin string, args []string) (string, error) {
		calls = append(calls, capturedCmd{label, dir, bin, args})
		// Materialize the artifact the downstream stage checks for.
		switch label {
		case "export-extract":
			_ = os.MkdirAll(filepath.Join(cfg.WorkDir, "x"), 0o750)
			_ = os.WriteFile(filepath.Join(cfg.WorkDir, "x", "llm_interactions.jsonl"), []byte("{}\n"), 0o600)
		case "curate":
			v := filepath.Join(cfg.WorkDir, "curated", "sft_interactions", "versioned")
			_ = os.MkdirAll(v, 0o750)
			_ = os.WriteFile(filepath.Join(v, "train.jsonl"), []byte("{}\n"), 0o600)
			_ = os.WriteFile(filepath.Join(v, "val.jsonl"), []byte("{}\n"), 0o600)
		case "train":
			_ = os.WriteFile(filepath.Join(cfg.WorkDir, "adapter", "adapters.safetensors"), []byte("x"), 0o600)
		case "convert-quantize":
			_ = os.WriteFile(filepath.Join(cfg.WorkDir, "candidate-Q5_K_M.gguf"), []byte("x"), 0o600)
		}
		return "", nil
	}
	return c, &calls
}

// TestStageCommands_Construction validates each runCmd-based stage builds the
// proven Epic-6 command + threads its artifact.
func TestStageCommands_Construction(t *testing.T) {
	work := t.TempDir()
	cfg := ControllerConfig{
		WorkDir: work, RepoDir: "/repo", SpaceID: "mdemg-dev",
		BaseModel: ".local-models/base", BaseSHA: "abc123",
		UaitsSpec: "docs/x.uaits.json", BenchmarkConfig: "configs/b.yaml",
		LoraRank: 32, LoraAlpha: 64, EpochsCap: 3, ExportSinceDays: 7,
		PythonBin: "python3", MdemgBin: "/bin/mdemg",
	}
	c, calls := newCaptureController(t, cfg)
	ctx := context.Background()

	inputDir, err := c.stageExport(ctx, work)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	versioned, err := c.stageCurate(ctx, "cyc-1", work, inputDir)
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	// E6-10: valid.jsonl created from val.jsonl
	if _, err := os.Stat(filepath.Join(versioned, "valid.jsonl")); err != nil {
		t.Error("curate should create valid.jsonl from val.jsonl (E6-10)")
	}
	adapter, err := c.stageTrain(ctx, work, versioned)
	if err != nil {
		t.Fatalf("train: %v", err)
	}
	fused, err := c.stageFuse(ctx, work, adapter)
	if err != nil {
		t.Fatalf("fuse: %v", err)
	}
	if _, err := c.stageConvert(ctx, work, fused); err != nil {
		t.Fatalf("convert: %v", err)
	}

	joined := map[string]string{}
	for _, cc := range *calls {
		joined[cc.label] = cc.bin + " " + strings.Join(cc.args, " ")
	}
	// Export uses the mdemg binary.
	if !strings.Contains(joined["export"], "/bin/mdemg data export") || !strings.Contains(joined["export"], "--tables llm_interactions") {
		t.Errorf("export cmd wrong: %s", joined["export"])
	}
	// Curate: paradigm_router with the spec + cycle version.
	if !strings.Contains(joined["curate"], "-m training.paradigm_router") || !strings.Contains(joined["curate"], "--version cyc-1") {
		t.Errorf("curate cmd wrong: %s", joined["curate"])
	}
	// Train: the base SHA pin (E6-8) + rank/alpha/epochs.
	if !strings.Contains(joined["train"], "--expected-sha256 abc123") || !strings.Contains(joined["train"], "--rank 32") || !strings.Contains(joined["train"], "--n-epochs 3") {
		t.Errorf("train cmd wrong: %s", joined["train"])
	}
	// Convert: fuse --dequantize (E6-14) → gguf → quantize Q5_K_M.
	if !strings.Contains(joined["convert-fuse"], "mlx_lm.fuse") || !strings.Contains(joined["convert-fuse"], "--dequantize") {
		t.Errorf("convert-fuse cmd wrong: %s", joined["convert-fuse"])
	}
	if !strings.Contains(joined["convert-quantize"], "llama-quantize") || !strings.Contains(joined["convert-quantize"], "Q5_K_M") {
		t.Errorf("convert-quantize cmd wrong: %s", joined["convert-quantize"])
	}
}

func TestReadGateReport(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "r.json")
	_ = os.WriteFile(p, []byte(`{"aggregate_weighted_score":0.84,"truncated_rows":0}`), 0o600)
	score, trunc, err := readGateReport(p)
	if err != nil || score != 0.84 || trunc != 0 {
		t.Errorf("got score=%v trunc=%v err=%v, want 0.84/0/nil", score, trunc, err)
	}
}

func TestController_DiskFloorBlocks(t *testing.T) {
	c := NewController(nil, nil, nil, ControllerConfig{
		Enabled: true, LeasePath: filepath.Join(t.TempDir(), "l"), LeaseMax: time.Hour,
		MinFreeDiskGB: 1e9, RepoDir: t.TempDir(), WorkDir: t.TempDir(),
	})
	ran := false
	c.runCmd = func(context.Context, string, string, string, []string) (string, error) { ran = true; return "", nil }
	c.runCycle(context.Background(), "c", "v")
	if ran {
		t.Error("no stage command should run below the disk floor")
	}
}

// FTLOOP-DRILL-001: tool resolution must survive the launchd minimal PATH
// (the dockerbin class) — override wins, then PATH, then well-known bins.
func TestResolveTool(t *testing.T) {
	if got := resolveTool("anything", "/explicit/override"); got != "/explicit/override" {
		t.Errorf("override: got %q", got)
	}
	// "sh" is always on PATH — LookPath branch returns an absolute path.
	if got := resolveTool("sh", ""); got == "sh" {
		t.Errorf("PATH resolution failed for sh: got bare name")
	}
	// A nonexistent tool falls through to the bare name (execCmd surfaces the error).
	if got := resolveTool("definitely-not-a-real-tool-xyz", ""); got != "definitely-not-a-real-tool-xyz" {
		t.Errorf("fallthrough: got %q", got)
	}
}
