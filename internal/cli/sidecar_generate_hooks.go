package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/sidecar"
)

func newSidecarGenerateHooksCmd() *cobra.Command {
	var (
		format string
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "generate-hooks",
		Short: "Generate project-scoped session-start hook",
		Long: `Generate a session-start hook script with sidecar config values substituted.

The generated script uses the endpoint and space_id from the sidecar
configuration instead of hardcoded values, enabling project-scoped
CMS memory.

Examples:
  mdemg sidecar generate-hooks
  mdemg sidecar generate-hooks --dry-run
  mdemg sidecar generate-hooks --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSidecarGenerateHooks(format, dryRun)
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be generated without writing")

	return cmd
}

func runSidecarGenerateHooks(format string, dryRun bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	stateBefore := sidecar.CurrentStateFrom(cwd)

	// State guard: must be at least installed
	switch stateBefore {
	case sidecar.StateInstalled, sidecar.StateRunning, sidecar.StateStopped, sidecar.StateDegraded:
		// OK
	default:
		report := sidecar.NewReportEnvelope("mdemg sidecar generate-hooks", stateBefore, stateBefore, sidecar.ExitValidation)
		report.Issues = append(report.Issues, sidecar.ReportIssue{
			Code:        "INVALID_STATE",
			Severity:    "error",
			Message:     fmt.Sprintf("Cannot generate hooks in state %q", stateBefore),
			Remediation: "Run: mdemg sidecar init && mdemg sidecar install",
		})
		if format == "json" {
			return sidecar.PrintJSON(report)
		}
		fmt.Printf("Error: cannot generate hooks in state %q\n", stateBefore)
		fmt.Println("  → Run: mdemg sidecar init && mdemg sidecar install")
		return nil
	}

	// Load config
	configPath := sidecar.FindConfigFileFrom(cwd)
	if configPath == "" {
		return fmt.Errorf("no sidecar.yaml found")
	}
	cfg, err := sidecar.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Derive values from config
	projectDir := filepath.Dir(filepath.Dir(configPath)) // .mdemg/sidecar.yaml → project root
	endpoint := cfg.Runtime.Endpoint
	spaceID := sidecar.ResolveSpaceID(cfg, projectDir)
	sessionID := "claude-core"

	// Generate script content
	script := generateSessionStartScript(endpoint, spaceID, sessionID)

	// Target path
	hookDir := filepath.Join(projectDir, ".claude", "hooks")
	hookPath := filepath.Join(hookDir, "session-start.sh")

	report := sidecar.NewReportEnvelope("mdemg sidecar generate-hooks", stateBefore, stateBefore, sidecar.ExitSuccess)

	if dryRun {
		report.Result = "dry-run"
		report.NextActions = []string{fmt.Sprintf("Would write %s", hookPath)}
		if format == "json" {
			return sidecar.PrintJSON(report)
		}
		fmt.Println("Dry Run: generate-hooks")
		fmt.Println("========================")
		fmt.Printf("  Endpoint:   %s\n", endpoint)
		fmt.Printf("  Space ID:   %s\n", spaceID)
		fmt.Printf("  Session ID: %s\n", sessionID)
		fmt.Printf("  Target:     %s\n", hookPath)
		fmt.Println()
		fmt.Println("--- Generated script ---")
		fmt.Print(script)
		fmt.Println("--- End ---")
		return nil
	}

	// Backup existing hook if present
	if _, statErr := os.Stat(hookPath); statErr == nil {
		backupDir := filepath.Join(projectDir, ".mdemg", "backups")
		if mkErr := os.MkdirAll(backupDir, 0755); mkErr == nil {
			ts := time.Now().Format("20060102T150405")
			backupPath := filepath.Join(backupDir, fmt.Sprintf("session-start.sh.%s", ts))
			if data, readErr := os.ReadFile(hookPath); readErr == nil {
				if wErr := os.WriteFile(backupPath, data, 0644); wErr == nil {
					report.Changes = append(report.Changes, sidecar.ReportChange{
						Path:       hookPath,
						Action:     "backed-up",
						BackupPath: backupPath,
					})
				}
			}
		}
	}

	// Ensure hook directory exists
	if mkErr := os.MkdirAll(hookDir, 0755); mkErr != nil {
		return fmt.Errorf("create hook directory: %w", mkErr)
	}

	// Write script
	if wErr := os.WriteFile(hookPath, []byte(script), 0755); wErr != nil { //nolint:gosec // executable hook script
		return fmt.Errorf("write hook script: %w", wErr)
	}

	report.Changes = append(report.Changes, sidecar.ReportChange{
		Path:   hookPath,
		Action: "created",
	})
	report.NextActions = []string{
		fmt.Sprintf("Hook written to %s", hookPath),
		"Verify: bash " + hookPath,
	}

	if format == "json" {
		return sidecar.PrintJSON(report)
	}

	fmt.Println("Generate Hooks")
	fmt.Println("==============")
	fmt.Printf("  Endpoint:   %s\n", endpoint)
	fmt.Printf("  Space ID:   %s\n", spaceID)
	fmt.Printf("  Session ID: %s\n", sessionID)
	fmt.Println()
	for _, c := range report.Changes {
		fmt.Printf("  %s: %s\n", c.Action, c.Path)
		if c.BackupPath != "" {
			fmt.Printf("    backup: %s\n", c.BackupPath)
		}
	}
	fmt.Println()
	for _, a := range report.NextActions {
		fmt.Printf("  → %s\n", a)
	}

	return nil
}

func generateSessionStartScript(endpoint, spaceID, sessionID string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
# Hook: SessionStart — restore CMS memory context on every session start
# Generated by: mdemg sidecar generate-hooks
# This runs automatically before Claude sees any user input.

set -euo pipefail

MDEMG_URL="${MDEMG_URL:-%s}"
SPACE_ID="%s"
SESSION_ID="%s"
MAX_OBS=10

# Check if MDEMG server is reachable
if ! curl -sf "${MDEMG_URL}/healthz" -o /dev/null --connect-timeout 2; then
  cat <<'EOF'
⚠ CMS DISCONNECTED — MDEMG server is not running.
Memory is unavailable. You are operating without persistent context.
Warn the user: "CMS unavailable — memory disconnected."
Do NOT make irreversible decisions without user confirmation.
EOF
  exit 0
fi

# Call resume endpoint
RESUME=$(curl -sf -X POST "${MDEMG_URL}/v1/conversation/resume" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"max_observations\":${MAX_OBS}}" \
  --connect-timeout 5 --max-time 10 2>/dev/null) || {
  echo "⚠ CMS resume failed — memory may be incomplete."
  exit 0
}

# Extract key fields
OBS_COUNT=$(echo "$RESUME" | jq -r '.observations | length // 0' 2>/dev/null || echo "0")
THEME_COUNT=$(echo "$RESUME" | jq -r '.themes | length // 0' 2>/dev/null || echo "0")
CONCEPT_COUNT=$(echo "$RESUME" | jq -r '.emergent_concepts | length // 0' 2>/dev/null || echo "0")
SUMMARY=$(echo "$RESUME" | jq -r '.summary // "No summary available"' 2>/dev/null || echo "No summary")

# Phase 80: Meta-cognitive anomaly detection
if [ "$OBS_COUNT" -eq 0 ] 2>/dev/null; then
  NODE_COUNT=$(curl -sf "${MDEMG_URL}/v1/memory/stats?space_id=${SPACE_ID}" \
    --connect-timeout 2 --max-time 3 2>/dev/null | jq -r '.memory_count // 0' 2>/dev/null || echo "0")

  if [ "$NODE_COUNT" -gt 0 ] 2>/dev/null; then
    cat <<'ANOMALY'

!! CRITICAL: MEMORY RETURNED EMPTY !!
╔══════════════════════════════════════════════════════════════╗
║  Resume returned 0 observations for active space.           ║
║  Space has data but nothing was retrieved.                  ║
║                                                             ║
║  MANDATORY INVESTIGATION:                                   ║
║    1. POST /v1/self-improve/assess                          ║
║       {"space_id":"<space>","tier":"micro"}                 ║
║    2. GET /v1/memory/stats?space_id=<space>                 ║
║                                                             ║
║  DO NOT PROCEED until investigated.                         ║
╚══════════════════════════════════════════════════════════════╝
ANOMALY

    curl -sf -X POST "${MDEMG_URL}/v1/self-improve/assess" \
      -H "Content-Type: application/json" \
      -d "{\"space_id\":\"${SPACE_ID}\",\"tier\":\"micro\"}" \
      --connect-timeout 3 --max-time 8 -o /dev/null 2>/dev/null &

    curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
      -H "Content-Type: application/json" \
      -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"content\":\"ANOMALY: Resume returned 0 observations but space has ${NODE_COUNT} nodes. Embedder or query failure suspected.\",\"obs_type\":\"error\",\"tags\":[\"anomaly\",\"empty-resume\"]}" \
      --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &
  fi
fi

# Build context injection
cat <<EOF
═══ CMS MEMORY RESTORED ═══
Observations: ${OBS_COUNT} | Themes: ${THEME_COUNT} | Concepts: ${CONCEPT_COUNT}
Summary: ${SUMMARY}
EOF

# Output recent observations as bullet points
if [ "$OBS_COUNT" -gt 0 ] 2>/dev/null; then
  echo ""
  echo "Recent observations:"
  echo "$RESUME" | jq -r '.observations[]? | "  • [\(.obs_type // "unknown")] \(.content // .summary // "no content")"' 2>/dev/null || true
fi

# Output themes
if [ "$THEME_COUNT" -gt 0 ] 2>/dev/null; then
  echo ""
  echo "Active themes:"
  echo "$RESUME" | jq -r '.themes[]? | "  • \(.name // "unnamed") (members: \(.member_count // 0))"' 2>/dev/null || true
fi

echo ""
echo "═══ END CMS CONTEXT ═══"

# Phase 80: Auto RSIC health display
RSIC_HEALTH=$(curl -sf -X POST "${MDEMG_URL}/v1/self-improve/assess" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\",\"tier\":\"micro\"}" \
  --connect-timeout 3 --max-time 8 2>/dev/null) || true

if [ -n "$RSIC_HEALTH" ]; then
  OVERALL=$(echo "$RSIC_HEALTH" | jq -r '.overall_health // "?"' 2>/dev/null || echo "?")
  RETRIEVAL=$(echo "$RSIC_HEALTH" | jq -r '.retrieval_quality // "?"' 2>/dev/null || echo "?")
  MEMORY=$(echo "$RSIC_HEALTH" | jq -r '.memory_health // "?"' 2>/dev/null || echo "?")
  EDGE=$(echo "$RSIC_HEALTH" | jq -r '.edge_health // "?"' 2>/dev/null || echo "?")
  LEARN_PHASE=$(echo "$RSIC_HEALTH" | jq -r '.learning_phase // "?"' 2>/dev/null || echo "?")
  ORPHAN_RATIO=$(echo "$RSIC_HEALTH" | jq -r '.orphan_ratio // "?"' 2>/dev/null || echo "?")

  echo ""
  cat <<EOF
═══ RSIC HEALTH ═══
Overall: ${OVERALL} | Retrieval: ${RETRIEVAL} | Memory: ${MEMORY} | Edge: ${EDGE}
Learning: ${LEARN_PHASE} | Orphan ratio: ${ORPHAN_RATIO}
═══ END RSIC HEALTH ═══
EOF

  HEALTH_NUM=$(echo "$OVERALL" | grep -oE '^[0-9.]+' || echo "1")
  if [ "$(echo "$HEALTH_NUM < 0.5" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    cat <<'DEGRADED'

!! DEGRADED HEALTH DETECTED !!
Investigation checklist:
  1. GET /v1/memory/stats?space_id=<space>
  2. GET /v1/learning/stats?space_id=<space>
  3. POST /v1/self-improve/cycle {"space_id":"<space>","tier":"meso"}
DEGRADED
  fi
fi

# Reinforce recalled observations via co-activation
if [ "$OBS_COUNT" -gt 0 ] 2>/dev/null; then
  curl -sf -X POST "${MDEMG_URL}/v1/conversation/observe" \
    -H "Content-Type: application/json" \
    -d "{\"space_id\":\"${SPACE_ID}\",\"session_id\":\"${SESSION_ID}\",\"content\":\"Session resumed. ${OBS_COUNT} observations, ${THEME_COUNT} themes, ${CONCEPT_COUNT} concepts recalled. Memory continuity maintained.\",\"obs_type\":\"context\",\"tags\":[\"session-resume\",\"auto-reinforce\"]}" \
    --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &
fi

# Trigger graduation check in background
curl -sf -X POST "${MDEMG_URL}/v1/conversation/graduate" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\":\"${SPACE_ID}\"}" \
  --connect-timeout 2 --max-time 5 -o /dev/null 2>/dev/null &
`, endpoint, spaceID, sessionID)
}
