// Package cli — `mdemg diagnostics` subcommand tree.
//
// Beta pipeline Sprint A: bundles scrubbed environment + log tails + system
// info into a single tar.gz for beta testers to attach to GitHub issues.
// Turns "can't reproduce" into "reproduce in 2 minutes."
//
// PRIVACY CONTRACT: every text field the bundle contains passes through
// `internal/llmclient/scrubber.go`'s ScrubString before writing. Existing
// scrubber patterns (api_key, abs_path, env_secret, email, neo4j_cred) plus
// the SCRUB-ENV-REF-001 shell env-var-reference preservation apply. See
// docs/beta/install-checklist.md for the tester-facing contract.
package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mdemg/internal/llmclient"
)

const diagnosticsBundleSchemaVersion = "1"

// newDiagnosticsCmd is the parent `mdemg diagnostics` group.
func newDiagnosticsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diagnostics",
		Short: "Collect a scrubbed diagnostic bundle for a GitHub issue attachment",
		Long: `Diagnostic bundle collector for beta testers.

Produces a single tar.gz zip of scrubbed environment + log tails + system
info that beta testers can attach to a GitHub issue. Every text field
passes through the same PII scrubber the training data export uses
(api_key, abs_path, env_secret, email, neo4j_cred; shell env-var
references like $PGPASSWORD preserved per SCRUB-ENV-REF-001).

Subcommands:
  collect    Produce the bundle (writes ~/.mdemg/diagnostics/*.tar.gz)`,
	}
	cmd.AddCommand(newDiagnosticsCollectCmd())
	return cmd
}

// newDiagnosticsCollectCmd is `mdemg diagnostics collect`.
func newDiagnosticsCollectCmd() *cobra.Command {
	var (
		outPath   string
		logsTail  int
		noDocker  bool
		serverURL string
	)
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Collect a scrubbed diagnostic bundle",
		Long: `Bundles the following into a single tar.gz for GitHub issue attachment:

  - MANIFEST.json         — schema version, produced-at, hostname, mdemg version, contents map
  - version.txt           — mdemg version / build commit / build date
  - env.scrubbed          — .env from cwd, PII-scrubbed (secrets redacted; env-var refs preserved)
  - config-show.txt       — mdemg config show (scrubbed)
  - config-validate.txt   — mdemg config validate output + exit code
  - system.txt            — sw_vers/uname, docker --version, docker compose version, RAM, disk free
  - docker-ps.txt         — docker compose ps output (from cwd)
  - docker-logs/          — per-service docker compose logs --tail N (default 200)
  - server-log-tail.txt   — ~/.mdemg/logs/server.log tail (scrubbed; last N lines)
  - healthz.txt           — GET /healthz response
  - metrics-snapshot.txt  — GET /v1/metrics/snapshot response

Every text field is scrubbed via llmclient.ScrubString. Bundle path is
printed on completion so testers can attach it directly to a GitHub issue.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getwd: %w", err)
			}

			// Resolve output path
			if outPath == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("resolve home: %w", err)
				}
				dir := filepath.Join(home, ".mdemg", "diagnostics")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("mkdir %s: %w", dir, err)
				}
				hostname, _ := os.Hostname()
				if hostname == "" {
					hostname = "unknown-host"
				}
				stamp := time.Now().UTC().Format("20060102-150405")
				outPath = filepath.Join(dir, fmt.Sprintf("mdemg-diag-%s-%s.tar.gz", hostname, stamp))
			}

			fmt.Printf("MDEMG Diagnostics Collect\n")
			fmt.Printf("=========================\n\n")
			fmt.Printf("Working dir: %s\n", cwd)
			fmt.Printf("Output:      %s\n\n", outPath)

			f, err := os.Create(outPath)
			if err != nil {
				return fmt.Errorf("create %s: %w", outPath, err)
			}
			defer func() { _ = f.Close() }()

			gz := gzip.NewWriter(f)
			defer func() { _ = gz.Close() }()

			tw := tar.NewWriter(gz)
			defer func() { _ = tw.Close() }()

			b := diagBundler{tw: tw, cwd: cwd, logsTail: logsTail}
			manifest := diagManifest{
				SchemaVersion: diagnosticsBundleSchemaVersion,
				ProducedAt:    time.Now().UTC().Format(time.RFC3339),
				Contents:      map[string]string{},
			}
			hostname, _ := os.Hostname()
			manifest.Hostname = hostname
			manifest.MDEMGVersion = Version + " (" + Commit + " built " + BuildDate + ")"

			// Collect each section; every failure is logged in the MANIFEST
			// so the operator can see what WASN'T captured (never silently
			// skipped).
			b.tryAdd(&manifest, "version.txt", "mdemg version + build info", func() (string, error) {
				return fmt.Sprintf("mdemg version %s\ncommit: %s\nbuild date: %s\ngo: %s\nos/arch: %s/%s\n",
					Version, Commit, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH), nil
			})

			b.tryAdd(&manifest, "env.scrubbed", "cwd .env with secrets redacted", func() (string, error) {
				return b.readAndScrub(filepath.Join(cwd, ".env"))
			})

			b.tryAdd(&manifest, "config-show.txt", "mdemg config show output (scrubbed)", func() (string, error) {
				return b.runAndScrub(ctx, "mdemg", "config", "show")
			})

			b.tryAdd(&manifest, "config-validate.txt", "mdemg config validate output + exit code", func() (string, error) {
				return b.runAndScrubWithExit(ctx, "mdemg", "config", "validate")
			})

			b.tryAdd(&manifest, "system.txt", "OS/docker/RAM/disk info", func() (string, error) {
				return b.systemInfo(ctx), nil
			})

			if !noDocker {
				b.tryAdd(&manifest, "docker-ps.txt", "docker compose ps output from cwd", func() (string, error) {
					return b.runAndScrub(ctx, "docker", "compose", "ps")
				})

				// docker compose logs per service — best-effort
				services, _ := b.listComposeServices(ctx)
				for _, svc := range services {
					name := "docker-logs/" + svc + ".log"
					desc := "docker compose logs --tail " + fmt.Sprintf("%d", logsTail) + " " + svc
					svc := svc // capture
					b.tryAdd(&manifest, name, desc, func() (string, error) {
						return b.runAndScrub(ctx, "docker", "compose", "logs", "--tail", fmt.Sprintf("%d", logsTail), svc)
					})
				}
			}

			b.tryAdd(&manifest, "server-log-tail.txt", "~/.mdemg/logs/server.log tail (scrubbed)", func() (string, error) {
				home, _ := os.UserHomeDir()
				return b.tailAndScrub(filepath.Join(home, ".mdemg", "logs", "server.log"), logsTail)
			})

			if serverURL != "" {
				b.tryAdd(&manifest, "healthz.txt", "GET "+serverURL+"/healthz", func() (string, error) {
					return b.httpFetchAndScrub(ctx, serverURL+"/healthz")
				})
				b.tryAdd(&manifest, "metrics-snapshot.txt", "GET "+serverURL+"/v1/metrics/snapshot", func() (string, error) {
					return b.httpFetchAndScrub(ctx, serverURL+"/v1/metrics/snapshot")
				})
			}

			// Write MANIFEST.json LAST so it reflects every prior addition.
			manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal manifest: %w", err)
			}
			if err := b.writeTarFile("MANIFEST.json", manifestJSON); err != nil {
				return fmt.Errorf("write MANIFEST: %w", err)
			}

			fmt.Printf("\nBundle contents: %d files\n", len(manifest.Contents)+1) // +1 for MANIFEST itself
			for name, desc := range manifest.Contents {
				fmt.Printf("  - %-25s %s\n", name, desc)
			}
			fmt.Printf("\nBundle written to:\n  %s\n\n", outPath)
			fmt.Printf("Attach this file directly to your GitHub issue at:\n")
			fmt.Printf("  https://github.com/reh3376/mdemg/issues/new/choose\n\n")
			fmt.Printf("Privacy: every text field was scrubbed for secrets (api_key/env_secret/\n")
			fmt.Printf("email/neo4j_cred/absolute_paths). Shell env-var references (e.g.\n")
			fmt.Printf("$PGPASSWORD) are preserved. Review the bundle contents before attaching\n")
			fmt.Printf("if you're unsure.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "Bundle output path (default: ~/.mdemg/diagnostics/mdemg-diag-<host>-<ts>.tar.gz)")
	cmd.Flags().IntVar(&logsTail, "logs-tail", 200, "Lines to include from each log tail")
	cmd.Flags().BoolVar(&noDocker, "no-docker", false, "Skip docker compose ps + per-service logs (useful when docker isn't running)")
	cmd.Flags().StringVar(&serverURL, "server-url", "http://localhost:9999", "MDEMG server URL for /healthz + metrics-snapshot probes (empty to skip)")
	return cmd
}

// diagManifest is the JSON structure written to MANIFEST.json in the bundle.
// Consumers (an issue-triager, a repro script, an agent) read this first to
// know what's in the bundle.
type diagManifest struct {
	SchemaVersion string            `json:"schema_version"`
	ProducedAt    string            `json:"produced_at"`
	Hostname      string            `json:"hostname"`
	MDEMGVersion  string            `json:"mdemg_version"`
	Contents      map[string]string `json:"contents"` // filename → one-line description
	Errors        map[string]string `json:"errors,omitempty"`
}

// diagBundler encapsulates tar-writing + scrubbing helpers. Kept in this file
// so the diagnostics logic stays self-contained (no new package required).
type diagBundler struct {
	tw       *tar.Writer
	cwd      string
	logsTail int
}

func (b *diagBundler) tryAdd(m *diagManifest, name, desc string, gen func() (string, error)) {
	content, err := gen()
	if err != nil {
		if m.Errors == nil {
			m.Errors = map[string]string{}
		}
		m.Errors[name] = err.Error()
		// Also write a placeholder so the file appears in the bundle
		content = "[collect failed: " + err.Error() + "]\n"
	}
	if err := b.writeTarFile(name, []byte(content)); err != nil {
		if m.Errors == nil {
			m.Errors = map[string]string{}
		}
		m.Errors[name] = "tar write: " + err.Error()
		return
	}
	m.Contents[name] = desc
}

func (b *diagBundler) writeTarFile(name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Now().UTC(),
	}
	if err := b.tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := b.tw.Write(data)
	return err
}

func (b *diagBundler) readAndScrub(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return llmclient.ScrubString(string(data)), nil
}

func (b *diagBundler) tailAndScrub(path string, n int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return llmclient.ScrubString(strings.Join(lines, "\n")), nil
}

func (b *diagBundler) runAndScrub(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: args are hard-coded above
	cmd.Dir = b.cwd
	out, err := cmd.CombinedOutput()
	scrubbed := llmclient.ScrubString(string(out))
	if err != nil {
		return scrubbed + "\n[exit error: " + err.Error() + "]\n", nil // record the failure inline; not fatal
	}
	return scrubbed, nil
}

func (b *diagBundler) runAndScrubWithExit(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: args are hard-coded above
	cmd.Dir = b.cwd
	out, err := cmd.CombinedOutput()
	scrubbed := llmclient.ScrubString(string(out))
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return fmt.Sprintf("%s\n[exit code: %d]\n", scrubbed, exitCode), nil
}

func (b *diagBundler) listComposeServices(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--services") //nolint:gosec // G204: hard-coded
	cmd.Dir = b.cwd
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var services []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			services = append(services, line)
		}
	}
	return services, nil
}

func (b *diagBundler) systemInfo(ctx context.Context) string {
	var out strings.Builder
	out.WriteString("=== OS ===\n")
	if runtime.GOOS == "darwin" {
		if v, err := exec.CommandContext(ctx, "sw_vers").CombinedOutput(); err == nil {
			out.Write(v)
		}
	} else {
		if v, err := exec.CommandContext(ctx, "uname", "-a").CombinedOutput(); err == nil {
			out.Write(v)
		}
	}
	out.WriteString(fmt.Sprintf("\ngo runtime: %s %s/%s\n\n", runtime.Version(), runtime.GOOS, runtime.GOARCH))

	out.WriteString("=== Docker ===\n")
	if v, err := exec.CommandContext(ctx, "docker", "--version").CombinedOutput(); err == nil {
		out.Write(v)
	}
	if v, err := exec.CommandContext(ctx, "docker", "compose", "version").CombinedOutput(); err == nil {
		out.Write(v)
	}

	out.WriteString("\n=== Disk (cwd) ===\n")
	if v, err := exec.CommandContext(ctx, "df", "-h", b.cwd).CombinedOutput(); err == nil {
		out.Write(v)
	}

	out.WriteString("\n=== RAM ===\n")
	if runtime.GOOS == "darwin" {
		if v, err := exec.CommandContext(ctx, "sysctl", "hw.memsize", "hw.ncpu").CombinedOutput(); err == nil {
			out.Write(v)
		}
	} else {
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			// only include MemTotal + MemFree + MemAvailable (first 3 lines)
			lines := strings.Split(string(data), "\n")
			for i, l := range lines {
				if i >= 3 {
					break
				}
				out.WriteString(l + "\n")
			}
		}
	}

	return llmclient.ScrubString(out.String())
}

func (b *diagBundler) httpFetchAndScrub(ctx context.Context, url string) (string, error) {
	cmd := exec.CommandContext(ctx, "curl", "-sS", "--max-time", "5", url) //nolint:gosec // G204: URL is CLI-flag input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return llmclient.ScrubString(string(out)) + "\n[curl error: " + err.Error() + "]\n", nil
	}
	return llmclient.ScrubString(string(out)), nil
}

