//go:build !darwin

package alert

import "context"

// MacOSBackend is a no-op stub on non-darwin platforms.
type MacOSBackend struct{}

func NewMacOSBackend(_ Severity) *MacOSBackend {
	return &MacOSBackend{}
}

func (b *MacOSBackend) Send(_ context.Context, _ Alert) error {
	return nil
}
