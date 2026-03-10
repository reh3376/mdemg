package cli

import (
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
		if !force {
			fmt.Println("Use --force to install the latest release anyway.")
			return nil
		}
	} else if currentVersion == latestVersion && !force {
		fmt.Println("\nAlready at the latest version.")
		return nil
	}

	if dryRun {
		fmt.Printf("\nUpdate available: %s → %s\n", Version, release.TagName)
		fmt.Println("Run without --dry-run to install.")
		return nil
	}

	// Find the right asset for this platform
	archiveName := fmt.Sprintf("mdemg_%s_%s_%s.tar.gz", latestVersion, runtime.GOOS, runtime.GOARCH)
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
	extractCmd := exec.Command("tar", "-xzf", archivePath, "-C", tmpDir)
	if err := extractCmd.Run(); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("extract: %w", err)
	}
	fmt.Println("ok")

	newBinary := filepath.Join(tmpDir, "mdemg")
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

	// Use shasum on macOS, sha256sum on Linux
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("shasum", "-a", "256", archivePath)
	} else {
		cmd = exec.Command("sha256sum", archivePath)
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("compute checksum: %w", err)
	}

	actualHash := strings.Fields(string(output))[0]
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
