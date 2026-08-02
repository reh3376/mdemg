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
		Short: "Jiminy guidance controls (JIMINY-MODE-001)",
	}
	cmd.AddCommand(newJiminyModeCmd())
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
