// Package cli — `mdemg adapter benchmark`
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultBenchTimeoutEnv = "MDEMG_BENCH_TIMEOUT_SEC"
	defaultBenchTimeoutSec = 3600
	defaultBenchmarkConfig = "configs/benchmark_phase10.yaml"
)

// signalCleanupBenchmark receives a SIGTERM/SIGINT (or a channel-close
// on normal shutdown), stops the bench-serve if it started, then closes
// `done` and re-raises the signal so the process exits with the expected
// signal-terminated status. If the channel closes (normal shutdown), the
// goroutine exits silently. Extracted for testability
// (ADAPTER-SWAP-STANDARDIZE-002 arch decision: signal delivery testing
// in Go is fragile; test the cleanup function directly).
func signalCleanupBenchmark(sigCh <-chan os.Signal, done chan<- struct{}, benchStarted *atomic.Bool, pidfilePath string, port int) {
	sig, ok := <-sigCh
	if !ok {
		// Normal shutdown — defer closed the channel
		close(done)
		return
	}
	fmt.Fprintf(os.Stderr, "\n== received %s: stopping bench-serve (port=%d) before re-raising\n", sig, port)
	if benchStarted.Load() {
		_ = stopBenchServe(pidfilePath, port)
	}
	close(done)
	// Re-raise the signal with default handler so the process exits
	// with a signal-terminated status (equivalent to shell's 128+N).
	if syscallSig, ok := sig.(syscall.Signal); ok {
		signal.Reset(syscallSig)
		_ = syscall.Kill(os.Getpid(), syscallSig)
	}
}

func newAdapterBenchmarkCmd() *cobra.Command {
	var (
		adapter    string
		iter       int
		configPath string
		out        string
		base       string
		port       int
		applyTSDB  bool
		modelName  string
		timeoutSec int
	)
	cmd := &cobra.Command{
		Use:   "benchmark",
		Short: "Atomic: freeze + bench-serve + run_benchmark.py + teardown",
		Long: `Full atomic orchestration for a single-checkpoint benchmark run.

Sequence:
  1. If --iter is set, freeze that checkpoint as adapters.safetensors
  2. Start bench-serve on --port (default 8103)
  3. Invoke python -m neural.benchmarks.run_benchmark --mlx-base-url http://127.0.0.1:<port>/v1 ...
  4. Regardless of subprocess outcome, stop bench-serve (defer-cleanup)
  5. Print benchmark JSON summary + exit non-zero if subprocess failed

⚠️ ORTHOGONAL to production llama-server on port 8102 — bench-serve uses an
alt port and NEVER touches launchd 'com.mdemg.llama-server'.

Example:
  mdemg adapter benchmark --adapter adapters/phase_e3_v1_base_v3 --iter 1200 \
                          --out /tmp/bench_smoke.json --apply-tsdb
`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if port <= 0 {
				port = defaultBenchServePort
			}
			if configPath == "" {
				configPath = defaultBenchmarkConfig
			}
			if timeoutSec <= 0 {
				if v := os.Getenv(defaultBenchTimeoutEnv); v != "" {
					if n, err := strconv.Atoi(v); err == nil && n > 0 {
						timeoutSec = n
					}
				}
			}
			if timeoutSec <= 0 {
				timeoutSec = defaultBenchTimeoutSec
			}

			absAdapter, err := resolveAdapterDir(adapter)
			if err != nil {
				return err
			}

			// Step 1: freeze if requested
			if iter > 0 {
				fmt.Printf("== freeze iter=%d\n", iter)
				if err := freezeAdapterInline(absAdapter, iter); err != nil {
					return fmt.Errorf("freeze: %w", err)
				}
			}

			// Step 2: bench-serve
			pidfilePath, err := benchServePidFile(port)
			if err != nil {
				return err
			}
			if base == "" {
				base = os.Getenv(defaultBenchServeBaseEnv)
			}
			if base == "" {
				base = defaultBenchServeBase
			}
			startupSec := defaultBenchServeStartup
			if v := os.Getenv(benchServeStartupEnv); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					startupSec = n
				}
			}

			// SIGTERM/SIGINT handler — Go's defer does NOT run on
			// signal-based termination. Without this, wrapper kill
			// (Bash tool 10-min ceiling, operator ^C, etc.) leaves a
			// stale pidfile at ~/.mdemg/bench-serve-<port>.json that
			// blocks the next `bench-serve` invocation until manually
			// removed. Live-caught in MDEMG-USAGE-LORA-001 Epic 4
			// (#145); fixed in ADAPTER-SWAP-STANDARDIZE-002 (#146).
			//
			// Contract: idempotent (stopBenchServe handles missing
			// pidfile gracefully); benchStarted guard prevents cleanup
			// firing before startBenchServe succeeds; signal is
			// re-raised with default handler after cleanup so the
			// process exits with the expected signal-terminated status.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
			var benchStarted atomic.Bool
			signalDone := make(chan struct{})
			go signalCleanupBenchmark(sigCh, signalDone, &benchStarted, pidfilePath, port)
			defer func() {
				signal.Stop(sigCh)
				close(sigCh)
				<-signalDone
			}()

			fmt.Printf("== bench-serve start (port=%d, adapter=%s)\n", port, absAdapter)
			if err := startBenchServe(pidfilePath, absAdapter, base, port, defaultBenchServeMaxTok, startupSec); err != nil {
				return fmt.Errorf("bench-serve: %w", err)
			}
			benchStarted.Store(true)

			// defer-cleanup: always stop bench-serve on normal return
			// (signal-based termination is handled by the goroutine above)
			defer func() {
				fmt.Printf("== bench-serve stop (port=%d)\n", port)
				_ = stopBenchServe(pidfilePath, port)
			}()

			// Step 3: run_benchmark.py
			if out == "" {
				return fmt.Errorf("--out is required")
			}
			outAbs, err := filepath.Abs(out)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(outAbs), 0o755); err != nil {
				return err
			}
			if modelName == "" {
				modelName = base
			}
			mlxURL := fmt.Sprintf("http://127.0.0.1:%d/v1", port)
			args := []string{
				"-m", "neural.benchmarks.run_benchmark",
				"--config", configPath,
				"--out", outAbs,
				"--mlx-base-url", mlxURL,
				"--mlx-model-name", modelName,
			}
			if applyTSDB {
				args = append(args, "--apply-tsdb")
			}
			pyBin := os.Getenv(benchServePythonEnv)
			if pyBin == "" {
				pyBin = defaultBenchServePython
			}
			fmt.Printf("== run_benchmark (timeout=%ds): %s %s\n", timeoutSec, pyBin, strings.Join(args, " "))
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
			defer cancel()
			runCmd := exec.CommandContext(ctx, pyBin, args...)
			runCmd.Stdout = os.Stdout
			runCmd.Stderr = os.Stderr
			runErr := runCmd.Run()

			// Step 4: print result JSON summary regardless of runErr
			if _, err := os.Stat(outAbs); err == nil {
				summary, sumErr := summarizeBenchmarkJSON(outAbs)
				if sumErr == nil {
					fmt.Println("\n== benchmark result summary")
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					_ = enc.Encode(summary)
				} else {
					fmt.Fprintf(os.Stderr, "warn: could not summarize %s: %v\n", outAbs, sumErr)
				}
			}
			if runErr != nil {
				return fmt.Errorf("run_benchmark subprocess failed: %w", runErr)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&adapter, "adapter", "", "adapter directory (required)")
	cmd.Flags().IntVar(&iter, "iter", 0, "checkpoint iter to freeze before serving (optional; 0 = use current adapters.safetensors)")
	cmd.Flags().StringVar(&configPath, "config", defaultBenchmarkConfig, "benchmark config yaml")
	cmd.Flags().StringVar(&out, "out", "", "output benchmark JSON path (required)")
	cmd.Flags().StringVar(&base, "base", "", "base model path (default: MDEMG_BENCH_SERVE_BASE env or .local-models/qwen3-14b-4bit-base)")
	cmd.Flags().IntVar(&port, "port", defaultBenchServePort, "bench-serve port")
	cmd.Flags().BoolVar(&applyTSDB, "apply-tsdb", false, "pass --apply-tsdb to run_benchmark")
	cmd.Flags().StringVar(&modelName, "mlx-model-name", "", "override --mlx-model-name for run_benchmark (defaults to --base)")
	cmd.Flags().IntVar(&timeoutSec, "timeout-sec", 0, "max seconds for the run_benchmark subprocess (env MDEMG_BENCH_TIMEOUT_SEC, default 3600)")
	_ = cmd.MarkFlagRequired("adapter")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

// freezeAdapterInline is the same freeze logic as the freeze subcommand
// but callable in-process (skips cobra flag ceremony).
func freezeAdapterInline(dir string, iter int) error {
	cp, err := findCheckpointByIter(dir, iter)
	if err != nil {
		return err
	}
	mainPath := filepath.Join(dir, adapterMainFile)
	backupPath := filepath.Join(dir, adapterBackupFile)

	var backupSHA string
	var backedUp bool
	if _, err := os.Stat(mainPath); err == nil {
		preSHA, err := sha256File(mainPath)
		if err != nil {
			return err
		}
		if err := copyFileForFreeze(mainPath, backupPath); err != nil {
			return err
		}
		backupSHA = preSHA
		backedUp = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := copyFileForFreeze(cp.Path, mainPath); err != nil {
		return err
	}
	postSHA, err := sha256File(mainPath)
	if err != nil {
		return err
	}
	if postSHA != cp.SHA256 {
		return fmt.Errorf("freeze verify failed: checkpoint sha=%s frozen sha=%s", cp.SHA256, postSHA)
	}
	row := freezeAuditRow{
		Iter:         cp.Iter,
		CheckpointSH: cp.SHA256,
		FrozenAt:     time.Now().UTC(),
	}
	if backedUp {
		row.BackupPath = backupPath
		row.BackupSHA = backupSHA
	}
	return appendFreezeAudit(filepath.Join(dir, freezeAuditFile), row)
}

// summarizeBenchmarkJSON extracts an at-a-glance view from a
// run_benchmark output JSON: aggregate score + per-task means.
func summarizeBenchmarkJSON(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	// run_benchmark writes {"aggregate_weighted_score": float, "per_task": {task: {"mean": ..., ...}}, ...}
	summary := map[string]any{}
	if agg, ok := raw["aggregate_weighted_score"]; ok {
		summary["aggregate_weighted_score"] = agg
	}
	if pt, ok := raw["per_task"].(map[string]any); ok {
		means := map[string]any{}
		for task, v := range pt {
			if vmap, ok := v.(map[string]any); ok {
				if m, ok := vmap["mean"]; ok {
					means[task] = m
				}
			}
		}
		summary["per_task_mean"] = means
	}
	if ri, ok := raw["run_id"]; ok {
		summary["run_id"] = ri
	}
	return summary, nil
}
