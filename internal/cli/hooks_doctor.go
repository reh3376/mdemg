// HOOKSYNC-001 — `mdemg hooks doctor`: one-shot triage for the hook channel.
//
// The per-prompt delivery channel had a months-long silent outage that only a
// manual audit caught (HOOKWIRE-001). Doctor makes the triage mechanical:
// installed-vs-template parity, settings registration, server reachability,
// a stdin-contract self-test with real-shape payloads, alert-file state, and
// the last prompt-context heartbeat age.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	hook_templates "mdemg/internal/cli/hook_templates"
	"mdemg/internal/tsdb"

	"github.com/spf13/cobra"
)

// doctorCheck is one named check result.
type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // PASS | FAIL | SKIP
	Detail string `json:"detail,omitempty"`
}

func newHooksDoctorCmd() *cobra.Command {
	var (
		spaceID   string
		serverURL string
		jsonOut   bool
	)

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the Claude Code hook channel",
		Long: `Diagnose the Claude Code hook channel in this project.

Checks: installed hooks vs embedded templates, settings registration,
server reachability, stdin contract self-test (real payload shapes),
alert-file state, and the last prompt-context heartbeat age.

Exits non-zero if any check FAILs (SKIPs do not fail the run).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			if spaceID == "" {
				spaceID = resolveSpaceID(cmd)
			}
			if spaceID == "" {
				spaceID = filepath.Base(cwd)
			}
			if serverURL == "" {
				serverURL = "http://localhost:9999"
			}

			checks := runDoctorChecks(cwd, spaceID, serverURL)

			failed := 0
			if jsonOut {
				out, _ := json.MarshalIndent(checks, "", "  ")
				fmt.Println(string(out))
				for _, c := range checks {
					if c.Status == "FAIL" {
						failed++
					}
				}
			} else {
				fmt.Println("MDEMG Hooks Doctor")
				fmt.Println("==================")
				for _, c := range checks {
					mark := "✓"
					switch c.Status {
					case "FAIL":
						mark = "✗"
						failed++
					case "SKIP":
						mark = "-"
					}
					fmt.Printf("  %s %-28s %-4s %s\n", mark, c.Name, c.Status, c.Detail)
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d check(s) failed", failed)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space ID substituted in installed hooks (default: resolved/dirname)")
	cmd.Flags().StringVar(&serverURL, "server-url", "", "MDEMG server URL (default: http://localhost:9999)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")

	return cmd
}

// runDoctorChecks executes all doctor checks and returns their results.
func runDoctorChecks(cwd, spaceID, serverURL string) []doctorCheck {
	var checks []doctorCheck

	// 1. Template parity: installed hook == embedded template with SPACE_ID
	// substituted (the CI gate's local twin).
	for _, hf := range claudeHookFiles() {
		name := "parity:" + hf.Template
		installed := filepath.Join(cwd, ".claude", "hooks", hf.Template)
		data, err := os.ReadFile(installed)
		if err != nil {
			checks = append(checks, doctorCheck{name, "FAIL", "not installed (mdemg hooks install --type claude)"})
			continue
		}
		tmpl, err := hook_templates.FS.ReadFile(hf.Template)
		if err != nil {
			checks = append(checks, doctorCheck{name, "FAIL", "template missing from binary: " + err.Error()})
			continue
		}
		want := strings.ReplaceAll(string(tmpl), "{{SPACE_ID}}", spaceID)
		if string(data) != want {
			checks = append(checks, doctorCheck{name, "FAIL", "drifted from template (reinstall with --force or sync the template)"})
			continue
		}
		checks = append(checks, doctorCheck{name, "PASS", ""})
	}

	// 2. Settings registration: every event's command references our hook file.
	checks = append(checks, checkDoctorRegistration(cwd))

	// 3. Server reachability.
	serverUp := false
	{
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(serverURL + "/healthz")
		switch {
		case err != nil:
			checks = append(checks, doctorCheck{"server:healthz", "FAIL", err.Error()})
		case resp.StatusCode != http.StatusOK:
			resp.Body.Close()
			checks = append(checks, doctorCheck{"server:healthz", "FAIL", resp.Status})
		default:
			resp.Body.Close()
			serverUp = true
			checks = append(checks, doctorCheck{"server:healthz", "PASS", serverURL})
		}
	}

	// 4. Stdin contract self-test (unix only; needs the server for output).
	if runtime.GOOS == "windows" {
		checks = append(checks, doctorCheck{"stdin:prompt-context", "SKIP", "self-test not supported on windows"})
	} else if !serverUp {
		checks = append(checks, doctorCheck{"stdin:prompt-context", "SKIP", "server unreachable"})
	} else {
		checks = append(checks, checkDoctorStdinContract(cwd))
	}

	// 5. Alert file readable + pending count.
	checks = append(checks, checkDoctorAlertFile())

	// 6. Last prompt-context heartbeat age (TSDB; SKIP when unreachable).
	checks = append(checks, checkDoctorHeartbeatAge())

	return checks
}

func checkDoctorRegistration(cwd string) doctorCheck {
	const name = "settings:registration"
	settingsPath := filepath.Join(cwd, ".claude", "settings.local.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return doctorCheck{name, "FAIL", "settings.local.json not readable: " + err.Error()}
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return doctorCheck{name, "FAIL", "settings.local.json invalid JSON"}
	}
	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil {
		return doctorCheck{name, "FAIL", "no hooks key in settings.local.json"}
	}
	var missing []string
	for _, hf := range claudeHookFiles() {
		raw, _ := json.Marshal(hooks[hf.Event])
		if !strings.Contains(string(raw), hf.Template) {
			missing = append(missing, hf.Event+"→"+hf.Template)
		}
	}
	if len(missing) > 0 {
		return doctorCheck{name, "FAIL", "unregistered: " + strings.Join(missing, ", ")}
	}
	return doctorCheck{name, "PASS", ""}
}

// checkDoctorStdinContract pipes a real-shape UserPromptSubmit payload through
// the installed prompt-context hook and asserts the always-present footer.
func checkDoctorStdinContract(cwd string) doctorCheck {
	const name = "stdin:prompt-context"
	hookPath := filepath.Join(cwd, ".claude", "hooks", "prompt-context.sh")
	payload := `{"session_id":"hooks-doctor","prompt":"mdemg hooks doctor stdin contract self-test payload"}`

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", hookPath)
	cmd.Stdin = strings.NewReader(payload)
	cmd.Dir = cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return doctorCheck{name, "FAIL", fmt.Sprintf("hook exited: %v — %s", err, doctorTrunc(out.String(), 120))}
	}
	// The synergy footer prints whenever the contract parses + the server is up.
	if !strings.Contains(out.String(), "synergy-meta") {
		return doctorCheck{name, "FAIL", "no synergy-meta footer — stdin contract or server path broken"}
	}
	return doctorCheck{name, "PASS", ""}
}

func checkDoctorAlertFile() doctorCheck {
	const name = "alerts:file"
	path := os.Getenv("ALERT_FILE_PATH")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".mdemg", "alerts", "current.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return doctorCheck{name, "SKIP", "no alert file (none fired yet)"}
	}
	var af struct {
		Alerts []struct {
			Cleared bool `json:"cleared"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal(data, &af); err != nil {
		return doctorCheck{name, "FAIL", "alert file unparseable"}
	}
	pending := 0
	for _, a := range af.Alerts {
		if !a.Cleared {
			pending++
		}
	}
	return doctorCheck{name, "PASS", fmt.Sprintf("%d pending / %d total", pending, len(af.Alerts))}
}

// checkDoctorHeartbeatAge reports the age of the newest hook:prompt-context
// row — the absence-detection heartbeat (HOOKSYNC-001).
func checkDoctorHeartbeatAge() doctorCheck {
	const name = "heartbeat:prompt-context"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := tsdb.NewClient(ctx, tsdbConfigFromEnv())
	if err != nil {
		return doctorCheck{name, "SKIP", "TSDB unreachable"}
	}
	defer client.Close()

	var age *float64
	err = client.Pool().QueryRow(ctx,
		`SELECT EXTRACT(EPOCH FROM now() - max(recorded_at))
		   FROM scheduled_job_events WHERE job_name = 'hook:prompt-context'`).Scan(&age)
	if err != nil {
		return doctorCheck{name, "SKIP", "query failed: " + err.Error()}
	}
	if age == nil {
		return doctorCheck{name, "FAIL", "no heartbeat rows ever — channel has never fired"}
	}
	return doctorCheck{name, "PASS", fmt.Sprintf("last fire %s ago", (time.Duration(*age) * time.Second).Round(time.Second))}
}

func doctorTrunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
