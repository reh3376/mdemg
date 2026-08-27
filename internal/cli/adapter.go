// Package cli — ADAPTER-SWAP-STANDARDIZE-001 top-level `mdemg adapter` command.
//
// Wraps LoRA adapter bench serving + checkpoint freezing + atomic benchmarking
// so operators don't repeat the 5-step manual dance PHASE-E3 flagged.
//
// Subcommands:
//   list        — enumerate saved checkpoints in an adapter dir
//   freeze      — cp 000NNNN_adapters.safetensors → adapters.safetensors
//   bench-serve — start/stop mlx_lm.server on an alt port for A/B benchmarking
//   benchmark   — atomic freeze + bench-serve + run_benchmark.py + teardown
//
// ⚠️ ORTHOGONAL to `mdemg model swap` / `mdemg model rollback`: those govern
// the PRODUCTION FUSED-GGUF llama-server on port 8102. This CLI is for BENCH
// mlx_lm.server on an alt port (default 8103). NEVER touches launchd
// `com.mdemg.llama-server`.
//
// Sprint: docs/development/adapter-swap-standardize-001/sprint_plan.md
package cli

import "github.com/spf13/cobra"

func newAdapterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adapter",
		Short: "Manage LoRA adapters for A/B benchmarking",
		Long: `Manage LoRA adapters for A/B benchmarking (bench-serve on alt port + freeze checkpoint + atomic benchmark).

⚠️ Orthogonal to 'mdemg model swap' — this CLI is for BENCH mlx_lm.server on
an alt port (default 8103), NOT the production llama-server on port 8102.
` + "`mdemg model swap`" + ` remains the path for shipped FUSED-GGUF adapters.

Subcommands:
  list        — enumerate saved checkpoints in an adapter dir
  freeze      — cp 000NNNN_adapters.safetensors → adapters.safetensors
  bench-serve — start/stop mlx_lm.server on an alt port
  benchmark   — atomic freeze + bench-serve + run_benchmark.py + teardown
`,
	}
	cmd.AddCommand(newAdapterListCmd())
	cmd.AddCommand(newAdapterFreezeCmd())
	cmd.AddCommand(newAdapterBenchServeCmd())
	cmd.AddCommand(newAdapterBenchmarkCmd())
	return cmd
}
