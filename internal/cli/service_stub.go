//go:build !darwin && !linux

package cli

import "fmt"

type stubServiceManager struct{}

func newPlatformServiceManager() serviceManager {
	return &stubServiceManager{}
}

func (m *stubServiceManager) Install(projectDir, mdemgBin, spaceID string) error {
	return fmt.Errorf("service management not supported on this platform (use 'mdemg start' for manual mode)")
}

func (m *stubServiceManager) Uninstall() error {
	return fmt.Errorf("service management not supported on this platform")
}

func (m *stubServiceManager) Status() error {
	return fmt.Errorf("service management not supported on this platform")
}

func (m *stubServiceManager) Restart() error {
	return fmt.Errorf("service management not supported on this platform")
}

func (m *stubServiceManager) Logs(follow bool) error {
	return fmt.Errorf("service management not supported on this platform")
}
