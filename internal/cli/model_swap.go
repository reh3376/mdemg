// FT-RECURSIVE-003 E2: `mdemg model swap` + `mdemg model rollback` — the
// operator surface over the fail-closed serving-swap primitive. Promotion
// (E3) drives the same primitive from `ft-loop promote`.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"mdemg/internal/config"
	"mdemg/internal/ftloop"
	"mdemg/internal/tsdb"
)

func servingConfigFromEnv(cfg config.Config, repoDir string) ftloop.ServingConfig {
	link := cfg.FtLoopServingSymlink
	if !filepath.IsAbs(link) {
		link = filepath.Join(repoDir, link)
	}
	return ftloop.ServingConfig{
		SymlinkPath:   link,
		PlistLabel:    cfg.FtLoopServingPlistLabel,
		HealthURL:     cfg.FtLoopServingHealthURL,
		HealthTimeout: time.Duration(cfg.FtLoopSwapHealthTimeoutSec) * time.Second,
	}
}

func newModelSwapCmd() *cobra.Command {
	var target, version, notes string
	var yes bool
	cmd := &cobra.Command{
		Use:   "swap",
		Short: "Fail-closed serving swap: retarget the serving symlink to a GGUF + restart llama-server",
		Long: `Atomically retargets the serving symlink (FT_LOOP_SERVING_SYMLINK) at
--target, kickstarts the serving LaunchAgent, and verifies health. If the
new model does not come back healthy within FT_LOOP_SWAP_HEALTH_TIMEOUT_SEC,
the previous target is restored automatically (fail-closed). Records the
swap in ft_model_versions when TSDB is reachable.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if target == "" {
				return fmt.Errorf("--target is required")
			}
			if !yes {
				return fmt.Errorf("this restarts production serving — re-run with --yes to confirm")
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			repoDir, _ := os.Getwd()
			sc := servingConfigFromEnv(cfg, repoDir)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			res, err := ftloop.SwapServing(ctx, sc, target)
			if err != nil {
				return fmt.Errorf("swap: %w (reverted=%v)", err, res.Reverted)
			}
			fmt.Printf("swap complete: %s -> %s\n", res.Previous, res.Target)

			recordSwapVersions(ctx, cfg, res, version, notes)
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "absolute path to the GGUF to serve")
	cmd.Flags().StringVar(&version, "version", "", "version label for ft_model_versions (default: derived from filename)")
	cmd.Flags().StringVar(&notes, "notes", "", "notes for the ft_model_versions row")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the production restart")
	return cmd
}

func newModelRollbackCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Roll serving back to the most recently superseded model version",
		Long: `Reads the previous (most recently superseded) row from ft_model_versions
and swaps serving back to it, fail-closed. The rolled-back-from version is
marked rolled_back; the restored version becomes active again.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !yes {
				return fmt.Errorf("this restarts production serving — re-run with --yes to confirm")
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			repoDir, _ := os.Getwd()
			sc := servingConfigFromEnv(cfg, repoDir)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			client, err := tsdb.NewClient(ctx, tsdbConfigFromEnv())
			if err != nil {
				return fmt.Errorf("connect TSDB (rollback needs the version history): %w", err)
			}
			defer client.Close()
			pool := client.Pool()

			prev, err := tsdb.PreviousModelVersion(ctx, pool)
			if err != nil {
				return fmt.Errorf("query previous version: %w", err)
			}
			if prev == nil {
				return fmt.Errorf("no superseded version in ft_model_versions — nothing to roll back to")
			}
			active, _ := tsdb.ActiveModelVersion(ctx, pool)

			res, err := ftloop.SwapServing(ctx, sc, prev.ModelPath)
			if err != nil {
				return fmt.Errorf("rollback swap: %w (reverted=%v)", err, res.Reverted)
			}
			fmt.Printf("rollback complete: %s -> %s (version %s)\n", res.Previous, res.Target, prev.Version)

			if active != nil {
				_ = tsdb.MarkModelVersionStatus(ctx, pool, active.Version, tsdb.ModelVersionRolledBack)
			}
			_ = tsdb.MarkModelVersionStatus(ctx, pool, prev.Version, tsdb.ModelVersionActive)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the production restart")
	return cmd
}

// recordSwapVersions best-effort records the swap outcome to
// ft_model_versions (supersede previous active, insert/refresh the new
// active row). TSDB unreachable = warn-only; the swap itself already
// happened and serving state is the source of truth.
func recordSwapVersions(ctx context.Context, cfg config.Config, res ftloop.SwapResult, version, notes string) {
	client, err := tsdb.NewClient(ctx, tsdbConfigFromEnv())
	if err != nil {
		fmt.Printf("WARN: ft_model_versions not recorded (TSDB unreachable): %v\n", err)
		return
	}
	defer client.Close()
	pool := client.Pool()

	if active, err := tsdb.ActiveModelVersion(ctx, pool); err == nil && active != nil && active.ModelPath != res.Target {
		_ = tsdb.MarkModelVersionStatus(ctx, pool, active.Version, tsdb.ModelVersionSuperseded)
	}
	if version == "" {
		version = filepath.Base(res.Target)
	}
	if err := tsdb.RecordModelVersion(ctx, pool, tsdb.ModelVersionRow{
		Version:   version,
		ModelPath: res.Target,
		BaseModel: cfg.FtLoopBaseModel,
		Status:    tsdb.ModelVersionActive,
		Notes:     notes,
	}); err != nil {
		fmt.Printf("WARN: ft_model_versions record failed: %v\n", err)
	}
}
