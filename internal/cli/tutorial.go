// ONBOARDING-TUTORIAL-SERVE-001 E5 — `mdemg tutorial` CLI entry point.
//
// Opens the embedded onboarding deck served at /ui/tutorial/ in the operator's
// default browser. Resolves the server endpoint via the shipped priority chain
// (MDEMG_ENDPOINT env > .mdemg.port file > LISTEN_ADDR > default).
//
// Flags:
//
//	--url      Print the URL and exit (no browser spawn). Useful in CI /
//	           headless envs, or when the operator wants to copy the URL.
//	--check    Probe /ui/tutorial/ with a short timeout and report whether
//	           the handler is reachable + enabled. Exits non-zero on failure
//	           so scripts can gate on it.
package cli

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mdemg/internal/config"
)

const (
	// tutorialPath is the embedded tutorial handler's mount point. Must stay
	// in sync with server.go's `/ui/tutorial/` registration.
	tutorialPath = "/ui/tutorial/"

	// tutorialDefaultEndpoint is the fallback used when ResolveEndpoint
	// finds no explicit configuration. Matches the shipped `:9999` default.
	tutorialDefaultEndpoint = "http://localhost:9999"

	// tutorialCheckTimeout bounds the --check probe so a hung server
	// doesn't wedge the CLI.
	tutorialCheckTimeout = 3 * time.Second
)

func newTutorialCmd() *cobra.Command {
	var (
		urlOnly  bool
		checkFl  bool
	)

	cmd := &cobra.Command{
		Use:   "tutorial",
		Short: "Open the MDEMG onboarding tutorial in your browser",
		Long: `Opens the embedded MDEMG onboarding deck (the 107-slide walkthrough
shipped by MDEMG-ONBOARDING-DECK-001 + ONBOARDING-TUTORIAL-SERVE-001)
in your default browser.

Requires an mdemg server running locally; endpoint is resolved from
MDEMG_ENDPOINT, .mdemg.port, LISTEN_ADDR, or the default (http://localhost:9999).

The tutorial handler is gated by TUTORIAL_ENABLED (default true). If
you've disabled it, this command will tell you how to re-enable.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint := config.ResolveEndpoint(tutorialDefaultEndpoint)
			url := strings.TrimRight(endpoint, "/") + tutorialPath

			if checkFl {
				return runTutorialCheck(cmd, url)
			}

			if urlOnly {
				fmt.Fprintln(cmd.OutOrStdout(), url)
				return nil
			}

			if err := openBrowser(url); err != nil {
				// Fall back to printing the URL — not a hard failure.
				// Operator can copy it manually.
				fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: could not auto-open browser (%v)\n", err)
				fmt.Fprintln(cmd.OutOrStdout(), url)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Opening %s\n", url)
			return nil
		},
	}

	cmd.Flags().BoolVar(&urlOnly, "url", false, "Print the URL and exit (no browser spawn)")
	cmd.Flags().BoolVar(&checkFl, "check", false, "Probe the tutorial handler and report reachability")

	return cmd
}

// runTutorialCheck probes /ui/tutorial/ with a short timeout and reports
// status to the operator. Exits non-zero via returned error on any failure.
func runTutorialCheck(cmd *cobra.Command, url string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), tutorialCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("tutorial unreachable at %s: %w (is the server running? try: mdemg start)", url, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		fmt.Fprintf(cmd.OutOrStdout(), "OK  %s is reachable (HTTP 200)\n", url)
		return nil
	case http.StatusServiceUnavailable:
		return fmt.Errorf("tutorial handler returned 503 — likely disabled; set TUTORIAL_ENABLED=true in .env and restart")
	default:
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
}

// openBrowser launches the OS-default browser against url. Returns an
// error if the launch command fails to start; a successful spawn does
// NOT guarantee the browser actually loaded (it's fire-and-forget).
func openBrowser(url string) error {
	var cmdArgs []string
	switch runtime.GOOS {
	case "darwin":
		cmdArgs = []string{"open", url}
	case "linux":
		cmdArgs = []string{"xdg-open", url}
	case "windows":
		// rundll32 is the historically-stable path; `start` is a shell
		// builtin and needs cmd /c, which quotes url badly.
		cmdArgs = []string{"rundll32", "url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	//nolint:gosec // G204: url comes from ResolveEndpoint + the fixed path
	// constant; both are config-sourced, not operator-untrusted input.
	launcher := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	return launcher.Start()
}
