// Package cli — mdemg jiminy commands (JIMINY-MODE-001).
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// newJiminyCmd creates the `mdemg jiminy` group.
func newJiminyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jiminy",
		Short: "Jiminy guidance controls (JIMINY-MODE-001 + JIMINY-ENFORCE-003)",
	}
	cmd.AddCommand(newJiminyModeCmd())
	cmd.AddCommand(newJiminyOverrideCmd())
	cmd.AddCommand(newJiminyConstraintCmd()) // JIMINY-INFORMATIONAL-CATEGORY-001
	return cmd
}

// newJiminyOverrideCmd creates the `mdemg jiminy override` group (JIMINY-ENFORCE-003).
// Escape-hatch for the enforcement arc — install time-boxed suppressions on
// specific constraint codes when Jiminy's classifier flags a false positive.
func newJiminyOverrideCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "override",
		Short: "Operator escape-hatch for Jiminy enforcement (JIMINY-ENFORCE-003)",
		Long: `Manage time-boxed constraint overrides.

  apply   — install an override for a specific constraint code
  list    — show all currently-active overrides
  revoke  — remove an active override before its scheduled expiry

Every override apply/revoke/expire is JSONL-audited at the path in
JIMINY_OVERRIDE_AUDIT_PATH (default ~/.mdemg/jiminy-overrides.jsonl).`,
	}
	cmd.AddCommand(newJiminyOverrideApplyCmd())
	cmd.AddCommand(newJiminyOverrideListCmd())
	cmd.AddCommand(newJiminyOverrideRevokeCmd())
	return cmd
}

func newJiminyOverrideApplyCmd() *cobra.Command {
	var (
		mdemgURL       string
		sessionID      string
		constraintCode string
		reason         string
		duration       string
	)
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Install a time-boxed override on a constraint code",
		RunE: func(cmd *cobra.Command, args []string) error {
			if mdemgURL == "" {
				mdemgURL = envOr("MDEMG_URL", "http://localhost:9999")
			}
			if sessionID == "" {
				sessionID = envOr("JIMINY_STRICT_DEFAULT_SESSION_ID", "claude-core")
			}
			if constraintCode == "" {
				return fmt.Errorf("--constraint required")
			}
			if reason == "" {
				return fmt.Errorf("--reason required (audit trail depends on it)")
			}
			d, err := time.ParseDuration(duration)
			if err != nil {
				return fmt.Errorf("--duration invalid: %w (use e.g. 15m, 1h)", err)
			}
			if d <= 0 {
				return fmt.Errorf("--duration must be positive")
			}
			payload, _ := json.Marshal(map[string]any{
				"session_id":      sessionID,
				"constraint_code": constraintCode,
				"reason":          reason,
				"duration_sec":    int(d.Seconds()),
			})
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Post(mdemgURL+"/v1/jiminy/override", "application/json", bytes.NewReader(payload))
			if err != nil {
				return fmt.Errorf("POST: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			}
			fmt.Printf("override applied: constraint=%s session=%s duration=%s reason=%q\n", constraintCode, sessionID, d, reason)
			return nil
		},
	}
	cmd.Flags().StringVar(&mdemgURL, "url", "", "MDEMG server URL")
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Session key (default: JIMINY_STRICT_DEFAULT_SESSION_ID or claude-core)")
	cmd.Flags().StringVar(&constraintCode, "constraint", "", "Constraint code to override (required)")
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for the override (required — audit trail)")
	cmd.Flags().StringVar(&duration, "duration", "15m", "Duration (e.g. 15m, 1h)")
	return cmd
}

func newJiminyOverrideListCmd() *cobra.Command {
	var (
		mdemgURL  string
		sessionID string
		jsonOut   bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List currently-active overrides",
		RunE: func(cmd *cobra.Command, args []string) error {
			if mdemgURL == "" {
				mdemgURL = envOr("MDEMG_URL", "http://localhost:9999")
			}
			u := mdemgURL + "/v1/jiminy/override"
			if sessionID != "" {
				u += "?session_id=" + sessionID
			}
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Get(u)
			if err != nil {
				return fmt.Errorf("GET: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			}
			if jsonOut {
				fmt.Println(string(body))
				return nil
			}
			var parsed struct {
				Data struct {
					Count     int `json:"count"`
					Overrides []struct {
						SessionID      string `json:"session_id"`
						ConstraintCode string `json:"constraint_code"`
						Reason         string `json:"reason"`
						AppliedAt      string `json:"applied_at"`
						ExpiresAt      string `json:"expires_at"`
					} `json:"overrides"`
				} `json:"data"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				return fmt.Errorf("parse: %w", err)
			}
			fmt.Printf("%d active override(s)\n", parsed.Data.Count)
			for _, o := range parsed.Data.Overrides {
				fmt.Printf("  session=%s constraint=%s expires_at=%s reason=%q\n",
					o.SessionID, o.ConstraintCode, o.ExpiresAt, o.Reason)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mdemgURL, "url", "", "MDEMG server URL")
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Filter to session (default: all)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit raw JSON")
	return cmd
}

func newJiminyOverrideRevokeCmd() *cobra.Command {
	var (
		mdemgURL       string
		sessionID      string
		constraintCode string
	)
	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke an active override before its scheduled expiry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if mdemgURL == "" {
				mdemgURL = envOr("MDEMG_URL", "http://localhost:9999")
			}
			if sessionID == "" {
				sessionID = envOr("JIMINY_STRICT_DEFAULT_SESSION_ID", "claude-core")
			}
			if constraintCode == "" {
				return fmt.Errorf("--constraint required")
			}
			payload, _ := json.Marshal(map[string]any{"session_id": sessionID, "constraint_code": constraintCode})
			req, _ := http.NewRequest(http.MethodDelete, mdemgURL+"/v1/jiminy/override", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("DELETE: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			}
			fmt.Printf("override revoked: constraint=%s session=%s\n", constraintCode, sessionID)
			return nil
		},
	}
	cmd.Flags().StringVar(&mdemgURL, "url", "", "MDEMG server URL")
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Session key")
	cmd.Flags().StringVar(&constraintCode, "constraint", "", "Constraint code to revoke (required)")
	return cmd
}

// newJiminyModeCmd creates the `mdemg jiminy mode` subcommand.
// `mdemg jiminy mode`              → read current mode
// `mdemg jiminy mode strict`       → set enforcement mode
// `mdemg jiminy mode suggest`      → set advisory mode
func newJiminyModeCmd() *cobra.Command {
	var (
		mdemgURL  string
		sessionID string
	)
	cmd := &cobra.Command{
		Use:   "mode [strict|suggest]",
		Short: "Read or set the Jiminy enforcement mode for a session",
		Long: `Read or set the Jiminy enforcement mode.

  strict  = enforce (block Write/Edit + alert user on WARNED+ constraint violations)
  suggest = advisory only (surface guidance, don't block)

No argument → prints current mode. Setting requires an argument.

Session key defaults to "claude-core" (the shipped JIMINY_STRICT_DEFAULT_SESSION_ID).
Override with --session-id when the operator uses a different key.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if mdemgURL == "" {
				mdemgURL = envOr("MDEMG_URL", "http://localhost:9999")
			}
			if sessionID == "" {
				sessionID = envOr("JIMINY_STRICT_DEFAULT_SESSION_ID", "claude-core")
			}

			if len(args) == 0 {
				return jiminyModeGet(mdemgURL, sessionID)
			}
			mode := args[0]
			if mode != "strict" && mode != "suggest" {
				return fmt.Errorf("mode must be 'strict' or 'suggest', got %q", mode)
			}
			return jiminyModeSet(mdemgURL, sessionID, mode == "strict")
		},
	}
	cmd.Flags().StringVar(&mdemgURL, "url", "", "MDEMG server URL (default: $MDEMG_URL or http://localhost:9999)")
	cmd.Flags().StringVar(&sessionID, "session-id", "", "Session key (default: $JIMINY_STRICT_DEFAULT_SESSION_ID or claude-core)")
	return cmd
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func jiminyModeGet(url, sessionID string) error {
	u := fmt.Sprintf("%s/v1/jiminy/strict?session_id=%s", url, sessionID)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return fmt.Errorf("GET %s: %w", u, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Data struct {
			Mode           string `json:"mode"`
			Strict         bool   `json:"strict"`
			SessionID      string `json:"session_id"`
			BootDefault    string `json:"boot_default"`
			DefaultSession string `json:"default_session"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("parse: %w (body: %s)", err, string(body))
	}
	fmt.Printf("session_id:       %s\n", parsed.Data.SessionID)
	fmt.Printf("mode:             %s (strict=%v)\n", parsed.Data.Mode, parsed.Data.Strict)
	fmt.Printf("boot default:     %s\n", parsed.Data.BootDefault)
	fmt.Printf("default session:  %s\n", parsed.Data.DefaultSession)
	return nil
}

func jiminyModeSet(url, sessionID string, enabled bool) error {
	u := fmt.Sprintf("%s/v1/jiminy/strict", url)
	payload, _ := json.Marshal(map[string]any{"session_id": sessionID, "enabled": enabled})
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("POST %s: %w", u, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	mode := "suggest"
	if enabled {
		mode = "strict"
	}
	fmt.Printf("Jiminy mode set to %q for session %q.\n", mode, sessionID)
	return nil
}
