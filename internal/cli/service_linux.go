//go:build linux

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
)

var systemdUnits = []string{
	"mdemg.service",
	"mdemg-rsic.service",
	"mdemg-rsic.timer",
}

type linuxServiceManager struct{}

func newPlatformServiceManager() serviceManager {
	return &linuxServiceManager{}
}

func (m *linuxServiceManager) Install(projectDir, mdemgBin, spaceID string) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	sourceDir := filepath.Join(projectDir, "packaging", "mdemg_linux", "systemd")

	for _, unit := range systemdUnits {
		src := filepath.Join(sourceDir, unit)
		if _, err := os.Stat(src); err != nil {
			fmt.Printf("  Skipping %s (not found in %s)\n", unit, sourceDir)
			continue
		}

		dest := filepath.Join("/etc/systemd/system", fmt.Sprintf("mdemg@%s.service", u.Username))
		if unit != "mdemg.service" {
			dest = filepath.Join("/etc/systemd/system", unit)
		}

		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", src, err)
		}

		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w (run with sudo?)", dest, err)
		}
		fmt.Printf("Installed %s\n", dest)
	}

	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	for _, unit := range systemdUnits {
		_ = exec.Command("systemctl", "enable", unit).Run()
		fmt.Printf("Enabled %s\n", unit)
	}

	return nil
}

func (m *linuxServiceManager) Uninstall() error {
	for _, unit := range systemdUnits {
		_ = exec.Command("systemctl", "disable", unit).Run()
		_ = exec.Command("systemctl", "stop", unit).Run()
		fmt.Printf("Disabled %s\n", unit)
	}
	return nil
}

func (m *linuxServiceManager) Status() error {
	fmt.Println("MDEMG Service Status (systemd)")
	fmt.Println("==============================")
	cmd := exec.Command("systemctl", "status", "mdemg", "mdemg-rsic.timer", "--no-pager")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
	return nil
}

func (m *linuxServiceManager) Restart() error {
	if err := exec.Command("systemctl", "restart", "mdemg").Run(); err != nil {
		return fmt.Errorf("systemctl restart mdemg: %w", err)
	}
	fmt.Println("Restarted mdemg service")
	return nil
}

func (m *linuxServiceManager) Logs(follow bool) error {
	args := []string{"-u", "mdemg", "--no-pager"}
	if follow {
		args = append(args, "-f")
	} else {
		args = append(args, "-n", "50")
	}
	cmd := exec.Command("journalctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RefreshInstalledDarwinServices is darwin-only; no-op elsewhere (the Linux
// analog — systemd unit refresh — already lives in upgrade.go).
func RefreshInstalledDarwinServices(_, _, _ string) (int, error) { return 0, nil }
