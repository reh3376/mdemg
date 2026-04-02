//go:build linux

package cli

import "syscall"

// detectSystemRAMBytes returns total system RAM in bytes on Linux.
func detectSystemRAMBytes() (uint64, error) {
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return 0, err
	}
	return info.Totalram * uint64(info.Unit), nil
}
