package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestEdgeBinaryName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"darwin", "arm64", "mdemg-darwin-arm64"},
		{"darwin", "amd64", "mdemg-darwin-amd64"},
		{"linux", "amd64", "mdemg-linux-amd64"},
		{"linux", "arm64", "mdemg-linux-arm64"},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			got := fmt.Sprintf("mdemg-%s-%s", tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("edge binary name = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStableArchiveName(t *testing.T) {
	tests := []struct {
		version, goos, goarch, want string
	}{
		{"0.4.0", "darwin", "arm64", "mdemg_0.4.0_darwin_arm64.tar.gz"},
		{"0.4.0", "linux", "amd64", "mdemg_0.4.0_linux_amd64.tar.gz"},
		{"0.4.0", "windows", "amd64", "mdemg_0.4.0_windows_amd64.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			ext := ".tar.gz"
			if tt.goos == "windows" {
				ext = ".zip"
			}
			got := fmt.Sprintf("mdemg_%s_%s_%s%s", tt.version, tt.goos, tt.goarch, ext)
			if got != tt.want {
				t.Errorf("stable archive name = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEdgeVersionComparison(t *testing.T) {
	tests := []struct {
		name          string
		currentCommit string
		edgeName      string
		wantSkip      bool
	}{
		{"same commit", "abc1234", "Edge Build (abc1234)", true},
		{"different commit", "abc1234", "Edge Build (def5678)", false},
		{"unknown commit", "unknown", "Edge Build (abc1234)", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the commit extraction logic from runUpgrade
			edgeCommit := tt.edgeName
			edgeCommit = edgeCommit[len("Edge Build ("):]
			edgeCommit = edgeCommit[:len(edgeCommit)-1]

			skip := tt.currentCommit != "unknown" && tt.currentCommit == edgeCommit
			if skip != tt.wantSkip {
				t.Errorf("skip = %v, want %v (current=%q edge=%q)", skip, tt.wantSkip, tt.currentCommit, edgeCommit)
			}
		})
	}
}

func TestCopyToBinDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	t.Run("bin dir exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Create a source binary
		srcPath := filepath.Join(tmpDir, "mdemg-src")
		if err := os.WriteFile(srcPath, []byte("binary content"), 0o755); err != nil {
			t.Fatalf("write src: %v", err)
		}

		destPath := filepath.Join(binDir, "mdemg")
		if err := copyFile(srcPath, destPath); err != nil {
			t.Fatalf("copyFile: %v", err)
		}
		if err := os.Chmod(destPath, 0o755); err != nil {
			t.Fatalf("chmod: %v", err)
		}

		// Verify file was copied
		if _, err := os.Stat(destPath); err != nil {
			t.Fatalf("dest not found: %v", err)
		}

		content, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("read dest: %v", err)
		}
		if string(content) != "binary content" {
			t.Errorf("content = %q, want %q", string(content), "binary content")
		}

		info, err := os.Stat(destPath)
		if err != nil {
			t.Fatalf("stat dest: %v", err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("permissions = %o, want 755", info.Mode().Perm())
		}
	})

	t.Run("no bin dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "bin")
		// bin dir does NOT exist
		if _, err := os.Stat(binDir); err == nil {
			t.Fatal("bin dir should not exist")
		}
	})
}

func TestUpgradeDockerInstances_NoDocker(t *testing.T) {
	// When Docker is not available, function should return silently
	// This test works in CI (no Docker) and verifies graceful degradation
	if DockerAvailable() {
		t.Skip("Docker is available — this test validates the no-Docker path")
	}
	// Should not panic, should not produce errors
	upgradeDockerInstances(context.Background(), "test-version")
}

func TestUpgradeDockerInstances_NoContainers(t *testing.T) {
	if !DockerAvailable() {
		t.Skip("Docker not available")
	}
	// 30s timeout prevents hanging if docker compose pull stalls in CI.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	upgradeDockerInstances(ctx, "test-version")
}

func TestRunUpgrade_DockerOnlyFlag(t *testing.T) {
	// Short timeout prevents hanging when real mdemg containers are running.
	oldTimeout := dockerUpgradeTimeout
	dockerUpgradeTimeout = 30 * time.Second
	defer func() { dockerUpgradeTimeout = oldTimeout }()

	// --docker-only should skip binary download entirely
	// and only call upgradeDockerInstances
	err := runUpgrade(false, false, false, false, true) // dockerOnly=true
	if err != nil {
		t.Errorf("runUpgrade with dockerOnly=true should not error: %v", err)
	}
}

func TestRunUpgrade_NoDockerDryRun(t *testing.T) {
	// Mock GitHub API to avoid rate limiting in CI.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v99.0.0", Name: "v99.0.0"})
	}))
	defer srv.Close()

	old := upgradeGitHubAPIURL
	upgradeGitHubAPIURL = srv.URL
	defer func() { upgradeGitHubAPIURL = old }()

	// --no-docker --dry-run should check for updates but skip Docker
	err := runUpgrade(true, false, false, true, false) // dryRun=true, noDocker=true
	if err != nil {
		t.Errorf("runUpgrade with noDocker+dryRun should not error: %v", err)
	}
}
