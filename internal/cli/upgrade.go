package cli

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	upgradeRepo         = "reh3376/mdemg"
	upgradeGitHubAPI    = "https://api.github.com/repos/" + upgradeRepo + "/releases/latest"
	upgradeDownloadBase = "https://github.com/" + upgradeRepo + "/releases/download"
)

func newUpgradeCmd() *cobra.Command {
	var dryRun bool
	var forceUpgrade bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Self-update the mdemg binary to the latest release",
		Long: `Download and install the latest mdemg release from GitHub.

Checks the current version against the latest release. If a newer version
is available, downloads the binary, verifies its checksum, and replaces
the current executable.

Use --dry-run to check for updates without installing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(dryRun, forceUpgrade)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Check for updates without installing")
	cmd.Flags().BoolVar(&forceUpgrade, "force", false, "Install even if already at latest version")

	return cmd
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func runUpgrade(dryRun, force bool) error {
	fmt.Printf("Current version: %s\n", Version)

	// Fetch latest release info
	fmt.Print("Checking for updates... ")
	release, err := fetchLatestRelease()
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("check for updates: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(Version, "v")
	fmt.Printf("latest is %s\n", release.TagName)

	if currentVersion == "dev" {
		fmt.Println("\nRunning development build — cannot compare versions.")
		if dryRun {
			fmt.Printf("\nLatest release: %s\n", release.TagName)
			fmt.Println("Run without --dry-run and with --force to install.")
			return nil
		}
		if !force {
			fmt.Println("Use --force to install the latest release anyway.")
			return nil
		}
	} else if currentVersion == latestVersion && !force {
		fmt.Println("\nAlready at the latest version.")
		return nil
	} else if dryRun {
		fmt.Printf("\nUpdate available: %s → %s\n", Version, release.TagName)
		fmt.Println("Run without --dry-run to install.")
		return nil
	}

	// Find the right asset for this platform
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	archiveName := fmt.Sprintf("mdemg_%s_%s_%s%s", latestVersion, runtime.GOOS, runtime.GOARCH, ext)
	checksumName := "checksums.txt"

	var archiveURL, checksumURL string
	for _, asset := range release.Assets {
		if asset.Name == archiveName {
			archiveURL = asset.BrowserDownloadURL
		}
		if asset.Name == checksumName {
			checksumURL = asset.BrowserDownloadURL
		}
	}

	if archiveURL == "" {
		return fmt.Errorf("no release binary found for %s/%s in %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	// Download to temp dir
	tmpDir, err := os.MkdirTemp("", "mdemg-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archivePath := filepath.Join(tmpDir, archiveName)
	fmt.Printf("Downloading %s... ", archiveName)
	if err := downloadFile(archiveURL, archivePath); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("download: %w", err)
	}
	fmt.Println("ok")

	// Verify checksum if available
	if checksumURL != "" {
		checksumPath := filepath.Join(tmpDir, checksumName)
		fmt.Print("Verifying checksum... ")
		if err := downloadFile(checksumURL, checksumPath); err != nil {
			fmt.Println("FAILED")
			return fmt.Errorf("download checksums: %w", err)
		}
		if err := verifyChecksum(archivePath, checksumPath, archiveName); err != nil {
			fmt.Println("FAILED")
			return fmt.Errorf("checksum verification: %w", err)
		}
		fmt.Println("ok")
	}

	// Extract
	fmt.Print("Extracting... ")
	if runtime.GOOS == "windows" {
		if err := extractZip(archivePath, tmpDir); err != nil {
			fmt.Println("FAILED")
			return fmt.Errorf("extract zip: %w", err)
		}
	} else {
		extractCmd := exec.Command("tar", "-xzf", archivePath, "-C", tmpDir)
		if err := extractCmd.Run(); err != nil {
			fmt.Println("FAILED")
			return fmt.Errorf("extract: %w", err)
		}
	}
	fmt.Println("ok")

	binaryName := "mdemg"
	if runtime.GOOS == "windows" {
		binaryName = "mdemg.exe"
	}
	newBinary := filepath.Join(tmpDir, binaryName)
	if _, err := os.Stat(newBinary); err != nil {
		return fmt.Errorf("binary not found in archive")
	}

	// Find current binary path
	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current binary: %w", err)
	}
	currentBinary, err = filepath.EvalSymlinks(currentBinary)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	// Replace current binary
	fmt.Printf("Installing to %s... ", currentBinary)

	// Backup current binary
	backupPath := currentBinary + ".backup"
	if err := os.Rename(currentBinary, backupPath); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("backup current binary: %w", err)
	}

	// Move new binary into place
	if err := copyFile(newBinary, currentBinary); err != nil {
		// Restore backup on failure
		_ = os.Rename(backupPath, currentBinary)
		fmt.Println("FAILED")
		return fmt.Errorf("install new binary: %w", err)
	}

	if err := os.Chmod(currentBinary, 0o755); err != nil { //nolint:gosec // Binary must be executable
		_ = os.Rename(backupPath, currentBinary)
		fmt.Println("FAILED")
		return fmt.Errorf("set permissions: %w", err)
	}
	fmt.Println("ok")

	// Clean up backup
	_ = os.Remove(backupPath)

	// Linux: update systemd unit files if present in archive and currently installed
	if runtime.GOOS == "linux" {
		systemdExtracted := filepath.Join(tmpDir, "systemd")
		if _, err := os.Stat(systemdExtracted); err == nil {
			systemdPaths := []string{"/etc/systemd/system", "/usr/lib/systemd/system"}
			sharePath := "/usr/local/share/mdemg/systemd"
			units := []struct{ src, dst string }{
				{"mdemg.service", "mdemg@.service"},
				{"mdemg-rsic.service", "mdemg-rsic@.service"},
				{"mdemg-rsic.timer", "mdemg-rsic@.timer"},
			}
			updated := false
			for _, systemdDir := range systemdPaths {
				// Only update if units already exist in this directory
				if _, err := os.Stat(filepath.Join(systemdDir, "mdemg@.service")); err != nil {
					continue
				}
				for _, u := range units {
					srcFile := filepath.Join(systemdExtracted, u.src)
					if _, err := os.Stat(srcFile); err == nil {
						if err := copyFile(srcFile, filepath.Join(systemdDir, u.dst)); err == nil {
							_ = os.Chmod(filepath.Join(systemdDir, u.dst), 0o644) //nolint:gosec // Systemd units must be world-readable
							updated = true
						}
					}
				}
			}
			// Also update the share directory copy
			if _, err := os.Stat(sharePath); err == nil {
				for _, u := range units {
					srcFile := filepath.Join(systemdExtracted, u.src)
					if _, err := os.Stat(srcFile); err == nil {
						_ = copyFile(srcFile, filepath.Join(sharePath, u.src))
						_ = os.Chmod(filepath.Join(sharePath, u.src), 0o644) //nolint:gosec // Systemd units must be world-readable
					}
				}
			}
			if updated {
				fmt.Print("Updating systemd units... ")
				if err := exec.Command("systemctl", "daemon-reload").Run(); err == nil {
					fmt.Println("ok")
				} else {
					fmt.Println("daemon-reload failed (may need sudo)")
				}
			}
		}
	}

	fmt.Printf("\nUpgraded mdemg to %s\n", release.TagName)
	return nil
}

func fetchLatestRelease() (*githubRelease, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(upgradeGitHubAPI)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parse release: %w", err)
	}
	return &release, nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func verifyChecksum(archivePath, checksumPath, archiveName string) error {
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}

	var expectedHash string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, archiveName) {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				expectedHash = parts[0]
				break
			}
		}
	}
	if expectedHash == "" {
		return fmt.Errorf("no checksum found for %s", archiveName)
	}

	// Use shasum on macOS, sha256sum on Linux, certutil on Windows
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("shasum", "-a", "256", archivePath)
	case "windows":
		cmd = exec.Command("certutil", "-hashfile", archivePath, "SHA256")
	default:
		cmd = exec.Command("sha256sum", archivePath)
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("compute checksum: %w", err)
	}

	var actualHash string
	if runtime.GOOS == "windows" {
		// certutil output: line 0 = header, line 1 = hash, line 2 = status
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) >= 2 {
			actualHash = strings.TrimSpace(lines[1])
		}
	} else {
		actualHash = strings.Fields(string(output))[0]
	}
	if actualHash != expectedHash {
		return fmt.Errorf("mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// extractZip extracts a .zip archive to destDir.
func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	const maxDecompressedSize = 100 << 20 // 100 MB safety limit

	for _, f := range r.File {
		target := filepath.Join(destDir, filepath.Clean(f.Name)) //nolint:gosec // Path is validated below
		// Prevent zip slip
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in archive: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		// Preserve executable permissions from the zip entry
		mode := f.Mode()
		if mode == 0 {
			mode = 0o644
		}

		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, io.LimitReader(rc, maxDecompressedSize)) //nolint:gosec // Size-limited via LimitReader
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
