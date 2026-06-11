package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/config"
	"mdemg/internal/tsdb"
)

// newModelCmd registers the `mdemg model` subcommand group. Subcommands
// delegate to the Fetcher interface (see model_fetcher.go); they have zero
// knowledge of backend-specific concepts. Adding a new backend means
// adding one file (e.g. model_fetcher_hf.go) and one case in NewFetcher —
// the CLI surface stays unchanged.
func newModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model",
		Short: "Manage local LLM models (Sprint MODEL-DIST-001 — pluggable distribution backends, Ollama default)",
		Long: `Manage local LLM models for the mdemg framework.

Every operator-visible value is dynamic — see the Configurability Contract
in docs/development/model-dist-001/sprint_plan_model_dist_001.md §3. Defaults
match the v1 production reality (Ollama Library, reh3376/mdemg-llm-v1, the
3 published quants Q4_K_M/Q5_K_M/Q8_0). Override via env or flag:

  MDEMG_MODEL_BACKEND    --backend         Distribution backend (default: ollama)
  MDEMG_MODEL_NAMESPACE  --namespace       Registry namespace (default: reh3376)
  MDEMG_MODEL_NAME       --name            Model name (default: mdemg-llm-v1)
  MDEMG_MODEL_QUANTS     --quants          Allowlist (default: Q4_K_M,Q5_K_M,Q8_0)
  MDEMG_MODEL_RAM_TIERS  --ram-tiers       JSON map for auto-pick
  MDEMG_MODEL_QUANT      --quant           Selected quant; 'auto' triggers RAM dispatch
  MDEMG_ADAPTER_BASE     --adapter-base    Base model for adapter Modelfile (adapter path shipped in MODEL-DIST-002)
  MDEMG_MODEL_DIR        --model-dir       Local symlink target dir (default: ~/.mdemg/models)
  MDEMG_MODEL_MANIFEST_PATH --manifest     Override embedded quant manifest
  OLLAMA_MODELS                            Ollama blob root (default: ~/.ollama/models)
  OLLAMA_HOST                              Ollama registry host (default: registry.ollama.ai)`,
	}
	cmd.AddCommand(newModelPullCmd())
	cmd.AddCommand(newModelListCmd())
	cmd.AddCommand(newModelVerifyCmd())
	cmd.AddCommand(newModelRemoveCmd())
	cmd.AddCommand(newModelWhereCmd())
	cmd.AddCommand(newModelRunCmd())
	return cmd
}

// modelOverrides is the set of CLI flag overrides for the Configurability
// Contract; populated by Cobra before each subcommand runs and merged into
// the resolved config in resolveModelConfig().
type modelOverrides struct {
	backend      string
	namespace    string
	name         string
	quants       string
	ramTiers     string
	quant        string
	adapterBase  string
	modelDir     string
	manifestPath string
}

// addOverrideFlags attaches the Configurability Contract flags to a command.
// Each flag mirrors an env var; flag takes precedence when set.
func (o *modelOverrides) addFlags(c *cobra.Command) {
	c.Flags().StringVar(&o.backend, "backend", "", "distribution backend (env MDEMG_MODEL_BACKEND)")
	c.Flags().StringVar(&o.namespace, "namespace", "", "registry namespace (env MDEMG_MODEL_NAMESPACE)")
	c.Flags().StringVar(&o.name, "name", "", "model name (env MDEMG_MODEL_NAME)")
	c.Flags().StringVar(&o.quants, "quants", "", "allowlist, comma-separated (env MDEMG_MODEL_QUANTS)")
	c.Flags().StringVar(&o.ramTiers, "ram-tiers", "", "JSON map for auto-pick (env MDEMG_MODEL_RAM_TIERS)")
	c.Flags().StringVar(&o.quant, "quant", "", "selected quant; 'auto' triggers RAM dispatch (env MDEMG_MODEL_QUANT)")
	c.Flags().StringVar(&o.adapterBase, "adapter-base", "", "adapter base model (env MDEMG_ADAPTER_BASE)")
	c.Flags().StringVar(&o.modelDir, "model-dir", "", "local symlink target dir (env MDEMG_MODEL_DIR)")
	c.Flags().StringVar(&o.manifestPath, "manifest", "", "override embedded quant manifest (env MDEMG_MODEL_MANIFEST_PATH)")
}

// resolveModelConfig loads env-derived config and overlays CLI flag values.
// This is the single point where flag → env → default precedence is enforced.
func (o *modelOverrides) resolve() (config.Config, error) {
	cfg, err := loadConfig()
	if err != nil {
		return cfg, err
	}
	if o.backend != "" {
		cfg.ModelBackend = o.backend
	}
	if o.namespace != "" {
		cfg.ModelNamespace = o.namespace
	}
	if o.name != "" {
		cfg.ModelName = o.name
	}
	if o.quants != "" {
		cfg.ModelQuants = o.quants
	}
	if o.ramTiers != "" {
		cfg.ModelRamTiers = o.ramTiers
	}
	if o.quant != "" {
		cfg.ModelQuant = o.quant
	}
	if o.adapterBase != "" {
		cfg.AdapterBase = o.adapterBase
	}
	if o.modelDir != "" {
		cfg.ModelDir = o.modelDir
	}
	if o.manifestPath != "" {
		cfg.ModelManifestPath = o.manifestPath
	}
	return cfg, nil
}

// recordModelEvent fires a V0021 model_install_events row best-effort: it
// times-out fast on connect failures and never blocks the CLI exit beyond the
// configured deadline (default 2s). Returning an error is non-fatal — the
// caller logs and proceeds.
//
// Empty/disabled TSDB config silently no-ops (per the framework's degraded-
// mode pattern: observability shouldn't block correctness).
func recordModelEvent(parent context.Context, cfg config.Config, row tsdb.ModelInstallEventRow) {
	if !cfg.TSDBEnabled || cfg.TSDBHost == "" {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	tsdbCfg := tsdb.Config{
		Host:     cfg.TSDBHost,
		Port:     cfg.TSDBPort,
		User:     cfg.TSDBUser,
		Password: cfg.TSDBPassword,
		Database: cfg.TSDBDatabase,
		SSLMode:  cfg.TSDBSSLMode,
		MaxConns: 1,
	}
	client, err := tsdb.NewClient(ctx, tsdbCfg)
	if err != nil {
		slog.Warn("model_install_events: TSDB connect failed (observability degraded)", "error", err)
		return
	}
	defer client.Close()
	if err := tsdb.RecordModelInstallEvent(ctx, client.Pool(), row); err != nil {
		slog.Warn("model_install_events: insert failed (observability degraded)", "error", err)
	}
}

// detectHostRAMGB returns total system RAM in GB (rounded down). darwin uses
// `sysctl hw.memsize`; linux parses /proc/meminfo's MemTotal. Other GOOS
// return (0, nil) so the resolver falls back to manual `--quant <q>`.
func detectHostRAMGB() (int, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0, fmt.Errorf("sysctl hw.memsize: %w", err)
		}
		bytesStr := strings.TrimSpace(string(out))
		bytes, err := strconv.ParseInt(bytesStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse hw.memsize %q: %w", bytesStr, err)
		}
		return int(bytes / (1024 * 1024 * 1024)), nil
	case "linux":
		raw, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0, fmt.Errorf("read /proc/meminfo: %w", err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.HasPrefix(line, "MemTotal:") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			kb, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse MemTotal %q: %w", fields[1], err)
			}
			return int(kb / (1024 * 1024)), nil
		}
		return 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
	default:
		return 0, nil
	}
}

// ─── pull ──────────────────────────────────────────────────────────────────

func newModelPullCmd() *cobra.Command {
	var o modelOverrides
	var adapter, dryRun bool
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull a fused-quant model from the configured backend",
		Long: `Pull mdemg-llm-v1 (or the operator-configured model) from the distribution
backend (default: Ollama Library). The flow:

  1. Resolve config from env + flags (Configurability Contract).
  2. Resolve quant: if 'auto', detect host RAM and dispatch via
     MDEMG_MODEL_RAM_TIERS; else use the explicit value (must be in
     MDEMG_MODEL_QUANTS allowlist).
  3. Call Fetcher.Fetch (Ollama: invoke 'ollama pull <namespace>/<name>:<quant>'
     then locate the GGUF blob via the manifest).
  4. Symlink the blob to <MDEMG_MODEL_DIR>/<name>.<quant>.gguf.
  5. SHA-verify against the embedded quant manifest (or
     MDEMG_MODEL_MANIFEST_PATH override).
  6. Print operator instructions for wiring into .env / restart.

Examples:
  mdemg model pull                                       # defaults — auto RAM dispatch
  mdemg model pull --quant Q4_K_M                        # explicit quant
  mdemg model pull --namespace acme --name custom-model  # fork's variant
  mdemg model pull --dry-run                             # plan only, no side effects`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := o.resolve()
			if err != nil {
				return err
			}
			return runModelPull(cmd.Context(), cfg, adapter, dryRun)
		},
	}
	o.addFlags(cmd)
	cmd.Flags().BoolVar(&adapter, "adapter", false, "pull adapter-only artifact (GGUF LoRA; shipped in MODEL-DIST-002)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "resolve config + print plan; no side effects")
	return cmd
}

func runModelPull(ctx context.Context, cfg config.Config, adapter, dryRun bool) error {
	hostRAMGB, ramErr := detectHostRAMGB()
	if ramErr != nil {
		// Soft-fail: if RAM detection fails, require explicit --quant.
		if strings.EqualFold(strings.TrimSpace(cfg.ModelQuant), "auto") {
			return fmt.Errorf("RAM auto-detection failed (%v) and MDEMG_MODEL_QUANT is 'auto'; specify --quant <Q4_K_M|Q5_K_M|Q8_0> explicitly", ramErr)
		}
		hostRAMGB = 0
	}

	quant, err := ResolveQuant(cfg, cfg.ModelQuant, hostRAMGB)
	if err != nil {
		return err
	}

	fetcher, err := NewFetcher(cfg)
	if err != nil {
		return err
	}

	req := FetchRequest{
		Namespace: cfg.ModelNamespace,
		Name:      cfg.ModelName,
		Quant:     quant,
		Adapter:   adapter,
		DestDir:   cfg.ModelDir,
		DryRun:    dryRun,
	}

	fmt.Printf("Resolved: backend=%s namespace=%s name=%s quant=%s adapter=%t host_ram_gb=%d dest=%s\n",
		fetcher.Name(), req.Namespace, req.Name, req.Quant, req.Adapter, hostRAMGB, req.DestDir)

	if dryRun {
		fmt.Println("(dry-run; no Fetch invoked)")
		return nil
	}

	result, err := fetcher.Fetch(ctx, req)
	if err != nil {
		// Record failure event before returning.
		recordModelEvent(ctx, cfg, tsdb.ModelInstallEventRow{
			EventType:   "pull",
			BackendName: fetcher.Name(),
			Namespace:   req.Namespace,
			ModelName:   req.Name,
			Quant:       req.Quant,
			Adapter:     req.Adapter,
			Success:     false,
			ErrMessage:  err.Error(),
		})
		return err
	}
	defer recordModelEvent(ctx, cfg, tsdb.ModelInstallEventRow{
		EventType:   "pull",
		BackendName: result.BackendName,
		Namespace:   req.Namespace,
		ModelName:   req.Name,
		Quant:       req.Quant,
		Adapter:     req.Adapter,
		Success:     true,
		LatencyMS:   result.LatencyMS,
		SHA256:      result.SHA256,
		SizeBytes:   result.SizeBytes,
	})

	// SHA verify against the quant manifest (embedded or operator-override).
	// Adapter pulls verify against mf.Adapter; fused quants against mf.Quants[quant].
	mf, mfErr := LoadQuantManifest(cfg)
	if mfErr == nil {
		var rec QuantRecord
		var ok bool
		var verifyKey string
		if adapter {
			if mf.Adapter != nil {
				rec = *mf.Adapter
				ok = true
			}
			verifyKey = "adapter"
		} else {
			rec, ok = mf.Quants[quant]
			verifyKey = quant
		}
		if ok && rec.SHA256 != "" {
			if !strings.EqualFold(result.SHA256, rec.SHA256) {
				return fmt.Errorf("SHA mismatch for %s: pulled blob has %s, quant manifest expects %s — do not trust this artifact", verifyKey, result.SHA256, rec.SHA256)
			}
			fmt.Printf("SHA verify: ok (%s)\n", rec.SHA256[:12]+"…")
		} else {
			fmt.Printf("SHA verify: skipped — quant manifest has no SHA recorded for %s yet\n", verifyKey)
		}
	} else {
		fmt.Printf("SHA verify: skipped — could not load quant manifest: %v\n", mfErr)
	}

	// Tag printout: fused = "<ns>/<name>:<quant>"; adapter = "<ns>/<name>-adapter:latest".
	tagDisplay := fmt.Sprintf("%s/%s:%s", req.Namespace, req.Name, req.Quant)
	if adapter {
		tagDisplay = fmt.Sprintf("%s/%s-adapter:latest", req.Namespace, req.Name)
	}
	fmt.Printf("Pulled %s → %s (%d bytes, %d ms)\n",
		tagDisplay, result.LocalPath, result.SizeBytes, result.LatencyMS)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Set in .env:  MDEMG_MODEL_PATH=%s\n", result.LocalPath)
	fmt.Println("  2. Restart llama-server: launchctl kickstart -k gui/$UID/com.mdemg.llama-server")
	fmt.Println("  3. Verify: curl -s http://127.0.0.1:8102/v1/models | jq .data[0].id")
	return nil
}

// ─── list ──────────────────────────────────────────────────────────────────

func newModelListCmd() *cobra.Command {
	var o modelOverrides
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List models pulled into <MDEMG_MODEL_DIR>",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := o.resolve()
			if err != nil {
				return err
			}
			return runModelList(cfg)
		},
	}
	o.addFlags(cmd)
	return cmd
}

func runModelList(cfg config.Config) error {
	entries, err := os.ReadDir(cfg.ModelDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("No models pulled (dir does not exist: %s)\n", cfg.ModelDir)
			return nil
		}
		return fmt.Errorf("read model dir %s: %w", cfg.ModelDir, err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSYMLINK_TARGET\tSIZE\tSHA256_PREFIX")
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".gguf") {
			continue
		}
		full := filepath.Join(cfg.ModelDir, e.Name())
		target, _ := os.Readlink(full)
		var size int64
		var shaPrefix string
		if info, err := os.Stat(full); err == nil {
			size = info.Size()
		}
		// Compute SHA only for files <1 GB or skip large; for list output,
		// we want it fast, so just show "(skipped: large)" for big files.
		if size < 100*1024*1024 {
			if h, err := sha256OfFile(full); err == nil {
				shaPrefix = h[:12]
			}
		} else {
			shaPrefix = "(use verify)"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", e.Name(), target, size, shaPrefix)
	}
	return w.Flush()
}

// ─── verify ────────────────────────────────────────────────────────────────

func newModelVerifyCmd() *cobra.Command {
	var o modelOverrides
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Re-check SHA256 of pulled models against the quant manifest",
		Long: `Walks <MDEMG_MODEL_DIR>/, computes SHA256 of each *.gguf symlink target,
and compares against the SHA recorded in the quant manifest (embedded into
the binary, or operator override via MDEMG_MODEL_MANIFEST_PATH).

Exit code 0 if all pulled artifacts match; 1 if any mismatch detected.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := o.resolve()
			if err != nil {
				return err
			}
			return runModelVerify(cfg)
		},
	}
	o.addFlags(cmd)
	return cmd
}

func runModelVerify(cfg config.Config) error {
	mf, err := LoadQuantManifest(cfg)
	if err != nil {
		return fmt.Errorf("load quant manifest: %w", err)
	}
	entries, err := os.ReadDir(cfg.ModelDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("No models pulled (dir does not exist: %s)\n", cfg.ModelDir)
			return nil
		}
		return err
	}
	failed := false
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".gguf") {
			continue
		}
		// Filename pattern: <name>.<quant>.gguf — parse to find quant.
		base := strings.TrimSuffix(e.Name(), ".gguf")
		idx := strings.LastIndex(base, ".")
		if idx < 0 {
			fmt.Printf("? %s — unexpected filename, skipping\n", e.Name())
			continue
		}
		quant := base[idx+1:]
		rec, ok := mf.Quants[quant]
		if !ok || rec.SHA256 == "" {
			fmt.Printf("? %s — no manifest record for quant %s, skipping\n", e.Name(), quant)
			continue
		}
		full := filepath.Join(cfg.ModelDir, e.Name())
		got, err := sha256OfFile(full)
		if err != nil {
			fmt.Printf("✗ %s — sha read failed: %v\n", e.Name(), err)
			failed = true
			continue
		}
		if !strings.EqualFold(got, rec.SHA256) {
			fmt.Printf("✗ %s — SHA mismatch: have %s, manifest %s\n", e.Name(), got, rec.SHA256)
			failed = true
			continue
		}
		fmt.Printf("✓ %s — sha %s\n", e.Name(), got[:16]+"…")
	}
	// Record a verify event. We use a single row for the whole walk (not one
	// per quant) — verify is a sweep operation, not per-artifact.
	recordModelEvent(context.Background(), cfg, tsdb.ModelInstallEventRow{
		EventType:   "verify",
		BackendName: cfg.ModelBackend,
		Namespace:   cfg.ModelNamespace,
		ModelName:   cfg.ModelName,
		Success:     !failed,
	})
	if failed {
		return fmt.Errorf("one or more SHA mismatches detected")
	}
	return nil
}

// ─── remove ────────────────────────────────────────────────────────────────

func newModelRemoveCmd() *cobra.Command {
	var o modelOverrides
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a pulled model (unlinks symlink + invokes backend Remove)",
		Long: `Removes the local symlink in <MDEMG_MODEL_DIR> AND invokes the backend's
Remove method (Ollama: 'ollama rm <namespace>/<name>:<quant>').

Requires --yes for the destructive action.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := o.resolve()
			if err != nil {
				return err
			}
			if !yes {
				return fmt.Errorf("refusing to remove without --yes (destructive: removes ollama-store copy AND local symlink)")
			}
			return runModelRemove(cmd.Context(), cfg)
		},
	}
	o.addFlags(cmd)
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the destructive action")
	return cmd
}

func runModelRemove(ctx context.Context, cfg config.Config) error {
	hostRAMGB, _ := detectHostRAMGB()
	quant, err := ResolveQuant(cfg, cfg.ModelQuant, hostRAMGB)
	if err != nil {
		return err
	}
	fetcher, err := NewFetcher(cfg)
	if err != nil {
		return err
	}
	req := FetchRequest{
		Namespace: cfg.ModelNamespace,
		Name:      cfg.ModelName,
		Quant:     quant,
		DestDir:   cfg.ModelDir,
	}
	rmErr := fetcher.Remove(ctx, req)
	row := tsdb.ModelInstallEventRow{
		EventType:   "remove",
		BackendName: fetcher.Name(),
		Namespace:   req.Namespace,
		ModelName:   req.Name,
		Quant:       req.Quant,
		Success:     rmErr == nil,
	}
	if rmErr != nil {
		row.ErrMessage = rmErr.Error()
	}
	recordModelEvent(ctx, cfg, row)
	return rmErr
}

// ─── where ─────────────────────────────────────────────────────────────────

func newModelWhereCmd() *cobra.Command {
	var o modelOverrides
	cmd := &cobra.Command{
		Use:   "where",
		Short: "Print the resolved local path for a pulled model (for shell scripting)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := o.resolve()
			if err != nil {
				return err
			}
			return runModelWhere(cfg)
		},
	}
	o.addFlags(cmd)
	return cmd
}

func runModelWhere(cfg config.Config) error {
	hostRAMGB, _ := detectHostRAMGB()
	quant, err := ResolveQuant(cfg, cfg.ModelQuant, hostRAMGB)
	if err != nil {
		return err
	}
	dest := filepath.Join(cfg.ModelDir, fmt.Sprintf("%s.%s.gguf", cfg.ModelName, quant))
	if _, err := os.Stat(dest); err != nil {
		return fmt.Errorf("%s not present (have you run 'mdemg model pull'?): %w", dest, err)
	}
	fmt.Println(dest)
	return nil
}

