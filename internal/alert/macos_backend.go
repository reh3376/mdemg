//go:build darwin

package alert

import (
	"context"
	"fmt"
	"os/exec"
)

// MacOSBackend sends macOS desktop notifications via osascript.
type MacOSBackend struct {
	minSeverity Severity
}

// NewMacOSBackend creates a macOS notification backend that only fires
// for alerts at or above the given minimum severity.
func NewMacOSBackend(minSeverity Severity) *MacOSBackend {
	return &MacOSBackend{minSeverity: minSeverity}
}

func (b *MacOSBackend) Send(ctx context.Context, a Alert) error {
	if !severityAtLeast(a.Severity, b.minSeverity) {
		return nil
	}
	script := fmt.Sprintf(`display notification %q with title "MDEMG Alert" subtitle %q`,
		a.Message, a.Title)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	return cmd.Run()
}

// severityAtLeast returns true if sev >= minSev in the severity ordering.
func severityAtLeast(sev, minSev Severity) bool {
	return severityRank(sev) >= severityRank(minSev)
}

func severityRank(s Severity) int {
	switch s {
	case SeverityLow:
		return 0
	case SeverityMedium:
		return 1
	case SeverityHigh:
		return 2
	case SeverityCritical:
		return 3
	default:
		return 0
	}
}
