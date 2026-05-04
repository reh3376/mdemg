package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"mdemg/internal/config"
)

// preflightMLXReachable refuses to start mdemg if mlx is not reachable.
// Hotfix 11.6.3.1 (2026-05-02): the always-on MLX policy
// (memory: feedback_mlx_required_when_mdemg_running.md) makes mlx a
// startup precondition because all 16 LLM call sites depend on it. Letting
// mdemg start with a dead mlx produces silent failures across half the
// framework — the policy makes that loud at startup instead.
//
// Operator escape hatch: MDEMG_ALLOW_NO_MLX=1 bypasses the check. Intended
// for Linux/Docker-only setups where the operator has wired LLM_ENDPOINT
// to a non-mlx provider, OR for emergency recovery when starting mdemg
// is required to investigate why mlx won't start.
//
// The probe is one HTTP GET against <LLM_ENDPOINT>/models with a 3s
// timeout. Failure modes (DNS not resolvable, connection refused, 5xx,
// timeout) all return an error that lists the resolved endpoint + the
// error reason + the escape-hatch instructions.
func preflightMLXReachable(cfg config.Config) error {
	if os.Getenv("MDEMG_ALLOW_NO_MLX") == "1" {
		// Operator opt-out — the policy explicitly allows this for
		// Linux/Docker-only setups + emergency recovery.
		return nil
	}

	endpoint := cfg.EffectiveLLMEndpoint()
	if endpoint == "" {
		return fmt.Errorf(
			"preflight: cfg.EffectiveLLMEndpoint() resolved empty — set LLM_ENDPOINT or OPENAI_ENDPOINT in .env (e.g. http://127.0.0.1:8101/v1 for native mlx_lm.server)\n" +
				"  Bypass for emergency recovery: MDEMG_ALLOW_NO_MLX=1 mdemg ...",
		)
	}

	probeURL := endpoint + "/models"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return fmt.Errorf("preflight: build mlx probe request to %s: %w", probeURL, err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf(
			"preflight: mlx unreachable at %s — mdemg requires mlx to be up at all times.\n"+
				"  Underlying error: %v\n"+
				"  Resolve via:\n"+
				"    1. Start mlx: `MDEMG_MLX_LM_BIN=/path/to/mlx_lm.server mdemg service install` then `launchctl kickstart gui/$(id -u)/com.mdemg.mlx-server`\n"+
				"    2. Or run mlx_lm.server manually: `mlx_lm.server --model %s --host 127.0.0.1 --port 8101 --prompt-cache-size 256 --prompt-concurrency 2 --decode-concurrency 2`\n"+
				"    3. (Emergency only) Bypass with `MDEMG_ALLOW_NO_MLX=1 mdemg ...`",
			probeURL, err, mlxModelHintFromEnv(),
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"preflight: mlx at %s returned HTTP %d (expected 2xx) — mdemg requires a healthy mlx.\n"+
				"  Bypass with MDEMG_ALLOW_NO_MLX=1 if intentional",
			probeURL, resp.StatusCode,
		)
	}
	return nil
}

// mlxModelHintFromEnv returns a sensible model-path hint for the error
// message. Falls back to a generic placeholder when the env var isn't set.
func mlxModelHintFromEnv() string {
	if v := os.Getenv("MDEMG_MODEL_PATH"); v != "" {
		return v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		return v
	}
	return "/path/to/.local-models/mdemg-llm-v1"
}
