package ftloop

import (
	"context"
	"strings"
	"testing"
)

// FT-RECURSIVE-004 E4: the recipe command must carry the
// BENCH-SIDECAR-APPLY-001 flag (rows land without manual psql) and the
// FT-BENCH-REFRESH-001 refresh calibration.
func TestBenchScheduler_CommandConstruction(t *testing.T) {
	var gotName string
	var gotArgs []string
	b := NewBenchScheduler(nil, BenchScheduleConfig{
		Enabled: true, RepoDir: "/repo", PythonBin: "/venv/python",
		ConfigYAML: "configs/b.yaml", BaseURL: "http://127.0.0.1:8102/v1",
		ModelName: "mdemg-llm-v1",
	}, nil)
	b.runCmd = func(_ context.Context, name string, args []string) error {
		gotName, gotArgs = name, args
		return nil
	}
	if err := b.runOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	joined := gotName + " " + strings.Join(gotArgs, " ")
	for _, w := range []string{"/venv/python", "-m neural.benchmarks.run_benchmark",
		"--apply-tsdb", "--rows-per-spec 5", "--n-runs 1", "--mlx-timeout-s 300",
		"--config /repo/configs/b.yaml"} {
		if !strings.Contains(joined, w) {
			t.Errorf("command missing %q: %s", w, joined)
		}
	}
}

func TestBenchScheduler_DisabledCompletes(t *testing.T) {
	b := NewBenchScheduler(nil, BenchScheduleConfig{Enabled: false}, nil)
	if err := b.Run(context.Background()); err != nil {
		t.Fatalf("disabled Run must return nil (intentional completion): %v", err)
	}
}
