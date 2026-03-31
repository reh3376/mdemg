//go:build windows

package api

import (
	"os"
	"os/exec"
)

// execRestart on Windows starts a new process and exits the current one.
func execRestart(binary string, args []string, env []string) error {
	cmd := exec.Command(binary, args[1:]...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
