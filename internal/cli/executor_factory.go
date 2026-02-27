package cli

import (
	"mdemg/internal/sidecar"
)

// newExecutor creates the appropriate Executor based on the sidecar config profile.
func newExecutor(cfg *sidecar.Config) (sidecar.Executor, error) {
	if cfg != nil && cfg.Profile == sidecar.ProfileStudioRemote {
		return newRemoteExecutor(cfg.Runtime.Remote.Host, cfg.Runtime.Remote.Transport)
	}
	return newLocalExecutor(), nil
}
