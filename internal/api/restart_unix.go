//go:build !windows

package api

import "syscall"

// execRestart replaces the current process with the given binary.
func execRestart(binary string, args []string, env []string) error {
	return syscall.Exec(binary, args, env)
}
