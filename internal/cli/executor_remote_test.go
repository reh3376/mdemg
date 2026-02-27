package cli

import (
	"testing"

	"mdemg/internal/sidecar"
)

func TestNewRemoteExecutor_RequiresHost(t *testing.T) {
	_, err := newRemoteExecutor("", sidecar.TransportDockerContext)
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

func TestNewRemoteExecutor_DefaultTransport(t *testing.T) {
	re, err := newRemoteExecutor("macstudio.local", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if re.transport != sidecar.TransportDockerContext {
		t.Errorf("expected docker-context transport, got %s", re.transport)
	}
}

func TestNewRemoteExecutor_SSHExecTransport(t *testing.T) {
	re, err := newRemoteExecutor("macstudio.local", sidecar.TransportSSHExec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if re.transport != sidecar.TransportSSHExec {
		t.Errorf("expected ssh-exec transport, got %s", re.transport)
	}
}

func TestRemoteExecutor_Host(t *testing.T) {
	re, err := newRemoteExecutor("macstudio-tb", sidecar.TransportDockerContext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if re.Host() != "macstudio-tb" {
		t.Errorf("expected macstudio-tb, got %s", re.Host())
	}
}

func TestRemoteExecutor_Close(t *testing.T) {
	re, err := newRemoteExecutor("macstudio.local", sidecar.TransportDockerContext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if closeErr := re.Close(); closeErr != nil {
		t.Errorf("expected nil close error, got %v", closeErr)
	}
}

func TestLocalExecutor_Host(t *testing.T) {
	le := newLocalExecutor()
	if le.Host() != "localhost" {
		t.Errorf("expected localhost, got %s", le.Host())
	}
}

func TestLocalExecutor_Close(t *testing.T) {
	le := newLocalExecutor()
	if closeErr := le.Close(); closeErr != nil {
		t.Errorf("expected nil close error, got %v", closeErr)
	}
}

func TestNewExecutor_LocalProfile(t *testing.T) {
	cfg := &sidecar.Config{Profile: sidecar.ProfileLocal}
	exec, err := newExecutor(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Host() != "localhost" {
		t.Errorf("expected localhost for local profile, got %s", exec.Host())
	}
}

func TestNewExecutor_NilConfig(t *testing.T) {
	exec, err := newExecutor(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Host() != "localhost" {
		t.Errorf("expected localhost for nil config, got %s", exec.Host())
	}
}

func TestNewExecutor_RemoteProfile(t *testing.T) {
	cfg := &sidecar.Config{
		Profile: sidecar.ProfileStudioRemote,
		Runtime: sidecar.RuntimeConfig{
			Endpoint: "http://macstudio-tb:9999",
			Remote: sidecar.RemoteConfig{
				Host:      "macstudio-tb",
				Transport: sidecar.TransportDockerContext,
			},
		},
	}
	exec, err := newExecutor(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Host() != "macstudio-tb" {
		t.Errorf("expected macstudio-tb for remote profile, got %s", exec.Host())
	}
}

func TestNewExecutor_RemoteProfileMissingHost(t *testing.T) {
	cfg := &sidecar.Config{
		Profile: sidecar.ProfileStudioRemote,
		Runtime: sidecar.RuntimeConfig{
			Remote: sidecar.RemoteConfig{
				Host: "",
			},
		},
	}
	_, err := newExecutor(cfg)
	if err == nil {
		t.Fatal("expected error for remote profile without host")
	}
}

func TestEnsureDockerContext_ConstName(t *testing.T) {
	if mdemgDockerContext != "mdemg-studio" {
		t.Errorf("expected mdemg-studio, got %s", mdemgDockerContext)
	}
}
