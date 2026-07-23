// FT-RECURSIVE-003 E2: the serving swap + rollback primitives.
//
// Production serving indirection: the llama-server plist points at ONE
// symlink (`FT_LOOP_SERVING_SYMLINK`, default
// .local-models/serving/current.gguf) and promotion/rollback retarget that
// symlink atomically then kickstart the LaunchAgent. Every swap is
// fail-closed: if the server does not come back healthy on the new target
// within the timeout, the symlink is retargeted back and the server
// restarted on the previous model — a bad candidate can not take down
// serving.
package ftloop

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ServingConfig describes the single-host serving layout.
type ServingConfig struct {
	SymlinkPath      string        // FT_LOOP_SERVING_SYMLINK
	PlistLabel       string        // FT_LOOP_SERVING_PLIST_LABEL (default com.mdemg.llama-server)
	HealthURL        string        // FT_LOOP_SERVING_HEALTH_URL (default http://127.0.0.1:8102/health)
	HealthTimeout    time.Duration // FT_LOOP_SWAP_HEALTH_TIMEOUT_SEC (default 120)
	KickstartCommand []string      // test seam; empty = real launchctl kickstart
}

// CurrentServingTarget resolves the symlink's current destination.
func CurrentServingTarget(cfg ServingConfig) (string, error) {
	dest, err := os.Readlink(cfg.SymlinkPath)
	if err != nil {
		return "", fmt.Errorf("serving symlink %s: %w (serving indirection not established — see ft-recursive-003 E2 cutover)", cfg.SymlinkPath, err)
	}
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(filepath.Dir(cfg.SymlinkPath), dest)
	}
	return dest, nil
}

// retargetSymlink atomically points cfg.SymlinkPath at target (tmp symlink +
// rename — atomic on the same filesystem).
func retargetSymlink(cfg ServingConfig, target string) error {
	tmp := cfg.SymlinkPath + ".swap.tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return fmt.Errorf("create temp symlink: %w", err)
	}
	if err := os.Rename(tmp, cfg.SymlinkPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic symlink rename: %w", err)
	}
	return nil
}

// kickstartServing restarts the serving LaunchAgent.
func kickstartServing(ctx context.Context, cfg ServingConfig) error {
	argv := cfg.KickstartCommand
	if len(argv) == 0 {
		argv = []string{"launchctl", "kickstart", "-k",
			fmt.Sprintf("gui/%s/%s", strconv.Itoa(os.Getuid()), cfg.PlistLabel)}
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // G204: constructed from config, not user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kickstart %s: %w (%s)", cfg.PlistLabel, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SwapResult reports what a swap did — the caller records it to the ledger
// and ft_model_versions.
type SwapResult struct {
	Previous string // symlink target before the swap
	Target   string // symlink target after the swap
	Reverted bool   // true when the fail-closed revert restored Previous
}

// SwapServing retargets serving at target with fail-closed health
// verification. On unhealthy-after-swap it reverts to the previous target,
// restarts, and returns an error (with Reverted=true in the result).
func SwapServing(ctx context.Context, cfg ServingConfig, target string) (SwapResult, error) {
	res := SwapResult{Target: target}

	st, err := os.Stat(target)
	if err != nil {
		return res, fmt.Errorf("swap target: %w", err)
	}
	if st.IsDir() || !strings.HasSuffix(target, ".gguf") {
		return res, fmt.Errorf("swap target %s is not a .gguf file", target)
	}

	prev, err := CurrentServingTarget(cfg)
	if err != nil {
		return res, err
	}
	res.Previous = prev
	if prev == target {
		slog.Info("serving swap: target already active", "target", target)
		return res, nil
	}

	if err := retargetSymlink(cfg, target); err != nil {
		return res, err
	}
	if err := kickstartServing(ctx, cfg); err != nil {
		// Symlink moved but restart failed — revert the link and try to bring
		// the previous model back before reporting.
		_ = retargetSymlink(cfg, prev)
		_ = kickstartServing(ctx, cfg)
		res.Reverted = true
		return res, fmt.Errorf("swap kickstart failed (reverted): %w", err)
	}

	timeout := cfg.HealthTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if err := waitHealth(ctx, cfg.HealthURL, timeout); err != nil {
		slog.Error("serving swap: candidate unhealthy — reverting", "target", target, "error", err)
		if rerr := retargetSymlink(cfg, prev); rerr != nil {
			return res, fmt.Errorf("candidate unhealthy AND revert failed: %v / %w", err, rerr)
		}
		if kerr := kickstartServing(ctx, cfg); kerr != nil {
			return res, fmt.Errorf("candidate unhealthy AND revert kickstart failed: %v / %w", err, kerr)
		}
		// The previous target IS restored at this point — record that truth
		// even if the trailing health wait below also times out.
		res.Reverted = true
		if herr := waitHealth(ctx, cfg.HealthURL, timeout); herr != nil {
			return res, fmt.Errorf("candidate unhealthy AND previous model slow to come back: %v / %w", err, herr)
		}
		return res, fmt.Errorf("swap reverted: candidate unhealthy: %w", err)
	}

	slog.Info("serving swap complete", "previous", prev, "target", target)
	return res, nil
}
