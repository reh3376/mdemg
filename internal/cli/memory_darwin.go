//go:build darwin

package cli

import "golang.org/x/sys/unix"

// detectSystemRAMBytes returns total system RAM in bytes on macOS.
func detectSystemRAMBytes() (uint64, error) {
	return unix.SysctlUint64("hw.memsize")
}
