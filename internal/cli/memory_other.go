//go:build !darwin && !linux

package cli

import "errors"

// detectSystemRAMBytes is unsupported on this platform.
func detectSystemRAMBytes() (uint64, error) {
	return 0, errors.New("RAM detection not supported on this platform")
}
