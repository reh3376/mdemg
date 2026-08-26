// Package cli — `mdemg adapter bench-serve`
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const (
	defaultBenchServePort    = 8103
	defaultBenchServeBaseEnv = "MDEMG_BENCH_SERVE_BASE"
	defaultBenchServeBase    = ".local-models/qwen3-14b-4bit-base"
	benchServeStartupEnv     = "MDEMG_BENCH_SERVE_STARTUP_TIMEOUT_SEC"
	defaultBenchServeStartup = 60
	defaultBenchServeMaxTok  = 4000
	// mlx_lm.server is installed in neural/.venv/ (per neural/pyproject.toml
	// [training] section). Operator override via MDEMG_BENCH_SERVE_PYTHON.
	benchServePythonEnv     = "MDEMG_BENCH_SERVE_PYTHON"
	defaultBenchServePython = "neural/.venv/bin/python"
)

func newAdapterBenchServeCmd() *cobra.Command {
	var (
		adapter    string
		base       string
		port       int
		maxTokens  int
		stop       bool
		startupSec int
	)
	cmd := &cobra.Command{
		Use:   "bench-serve",
		Short: "Start/stop mlx_lm.server on an alt port for A/B benchmarking",
		Long: `Start (or stop) a bench mlx_lm.server against a specified adapter directory.

⚠️ ORTHOGONAL to production llama-server on port 8102 — bench-serve uses an
alt port (default 8103) and NEVER touches launchd 'com.mdemg.llama-server'.

Start mode:
  mdemg adapter bench-serve --adapter adapters/phase_e3_v1_base_v3
  mdemg adapter bench-serve --adapter <dir> --port 8104 --base <base-model>

Stop mode:
  mdemg adapter bench-serve --stop [--port 8103]

Stop mode reads the pidfile at ~/.mdemg/bench-serve-<port>.json, sends SIGTERM
to the recorded PID, and removes the pidfile. Idempotent — no-op if pidfile absent.
`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if port <= 0 {
				port = defaultBenchServePort
			}
			pidfilePath, err := benchServePidFile(port)
			if err != nil {
				return err
			}

			if stop {
				return stopBenchServe(pidfilePath, port)
			}

			// Start mode
			if adapter == "" {
				return fmt.Errorf("--adapter is required in start mode")
			}
			absAdapter, err := resolveAdapterDir(adapter)
			if err != nil {
				return err
			}
			if base == "" {
				base = os.Getenv(defaultBenchServeBaseEnv)
			}
			if base == "" {
				base = defaultBenchServeBase
			}
			if startupSec <= 0 {
				if v := os.Getenv(benchServeStartupEnv); v != "" {
					if n, err := strconv.Atoi(v); err == nil && n > 0 {
						startupSec = n
					}
				}
			}
			if startupSec <= 0 {
				startupSec = defaultBenchServeStartup
			}
			return startBenchServe(pidfilePath, absAdapter, base, port, maxTokens, startupSec)
		},
	}
	cmd.Flags().StringVar(&adapter, "adapter", "", "adapter directory (required in start mode)")
	cmd.Flags().StringVar(&base, "base", "", "base model path (default: MDEMG_BENCH_SERVE_BASE env or .local-models/qwen3-14b-4bit-base)")
	cmd.Flags().IntVar(&port, "port", defaultBenchServePort, "port to bind bench-serve on")
	cmd.Flags().IntVar(&maxTokens, "max-tokens", defaultBenchServeMaxTok, "--max-tokens flag passed to mlx_lm.server")
	cmd.Flags().BoolVar(&stop, "stop", false, "stop the bench-serve on --port (or default 8103)")
	cmd.Flags().IntVar(&startupSec, "startup-timeout-sec", 0, "seconds to poll for readiness before giving up (env MDEMG_BENCH_SERVE_STARTUP_TIMEOUT_SEC)")
	return cmd
}

func startBenchServe(pidfilePath, adapterDir, base string, port, maxTokens, startupSec int) error {
	// Refuse if pidfile already exists — operator must --stop first
	if _, err := os.Stat(pidfilePath); err == nil {
		return fmt.Errorf("bench-serve pidfile already present at %s; run 'mdemg adapter bench-serve --stop --port %d' first", pidfilePath, port)
	}
	// Refuse if port already bound
	if err := checkPortFree(port); err != nil {
		return fmt.Errorf("port %d already in use: %w", port, err)
	}

	// Spawn mlx_lm.server as a detached background process
	args := []string{
		"-m", "mlx_lm.server",
		"--model", base,
		"--adapter-path", adapterDir,
		"--port", strconv.Itoa(port),
	}
	if maxTokens > 0 {
		args = append(args, "--max-tokens", strconv.Itoa(maxTokens))
	}
	pyBin := os.Getenv(benchServePythonEnv)
	if pyBin == "" {
		pyBin = defaultBenchServePython
	}
	cmd := exec.Command(pyBin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Log to file so operator can tail if desired
	dir, _ := mdemgStateDir()
	logPath := fmt.Sprintf("%s/bench-serve-%d.log", dir, port)
	logf, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // operator-private bench-serve log under ~/.mdemg/
	if err != nil {
		return fmt.Errorf("open bench-serve log: %w", err)
	}
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		logf.Close()
		return fmt.Errorf("spawn mlx_lm.server: %w", err)
	}
	logf.Close()

	rec := benchServePidRecord{
		PID:        cmd.Process.Pid,
		AdapterDir: adapterDir,
		BaseModel:  base,
		Port:       port,
		StartedAt:  time.Now().UTC(),
		MdemgCmd:   pyBin + " " + strings.Join(args, " "),
	}
	if err := writePidRecord(pidfilePath, rec); err != nil {
		// Best-effort kill the spawned pid to avoid orphaning
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		return fmt.Errorf("write pidfile: %w", err)
	}

	// Poll readiness
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/models", port)
	ready := false
	deadline := time.Now().Add(time.Duration(startupSec) * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:gosec // fixed localhost URL constructed from validated int port
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(2 * time.Second)
	}
	if !ready {
		// Kill spawned process + remove pidfile — cleanup so operator can retry
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		_ = os.Remove(pidfilePath)
		return fmt.Errorf("bench-serve failed to become ready within %ds — see %s for details", startupSec, logPath)
	}

	out := map[string]any{
		"status":      "up",
		"pid":         rec.PID,
		"port":        port,
		"url":         url,
		"adapter_dir": adapterDir,
		"base_model":  base,
		"pidfile":     pidfilePath,
		"log":         logPath,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func stopBenchServe(pidfilePath string, port int) error {
	rec, err := readPidRecord(pidfilePath)
	if os.IsNotExist(err) {
		fmt.Printf("no pidfile at %s — nothing to stop\n", pidfilePath)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read pidfile: %w", err)
	}
	// Kill the entire process group so mlx_lm.server's subprocesses go too
	if err := syscall.Kill(-rec.PID, syscall.SIGTERM); err != nil {
		if err == syscall.ESRCH {
			// Process already gone; still remove pidfile
			_ = os.Remove(pidfilePath)
			fmt.Printf("bench-serve pid %d already exited; pidfile removed\n", rec.PID)
			return nil
		}
		return fmt.Errorf("SIGTERM pid %d: %w", rec.PID, err)
	}
	// Wait briefly for graceful shutdown, then SIGKILL if still alive
	for i := 0; i < 5; i++ {
		if err := syscall.Kill(-rec.PID, 0); err == syscall.ESRCH {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	// Best-effort SIGKILL if still alive
	_ = syscall.Kill(-rec.PID, syscall.SIGKILL)

	if err := os.Remove(pidfilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pidfile: %w", err)
	}
	out := map[string]any{
		"status":  "stopped",
		"pid":     rec.PID,
		"port":    port,
		"pidfile": pidfilePath,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// checkPortFree returns nil if the TCP port is not currently bound.
func checkPortFree(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	return ln.Close()
}
