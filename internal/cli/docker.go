package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

const (
	neo4jContainerName = "mdemg-neo4j-dev"
	neo4jVolumeName    = "mdemg-neo4j-data"
	neo4jImage         = "neo4j:5"
	neo4jDefaultPort   = 7687
	neo4jDefaultHTTP   = 7474
)

// ContainerState holds information about a Docker container.
type ContainerState struct {
	Exists  bool
	Running bool
	Status  string // e.g. "running", "exited"
}

// DockerAvailable returns true if the docker CLI is available.
func DockerAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// InspectContainer returns the state of a named Docker container.
func InspectContainer(name string) (ContainerState, error) {
	out, err := RunDockerCommand("inspect", "--format", "{{.State.Status}}", name)
	if err != nil {
		// If the container doesn't exist, docker inspect returns an error
		return ContainerState{Exists: false}, nil
	}
	status := strings.TrimSpace(out)
	return ContainerState{
		Exists:  true,
		Running: status == "running",
		Status:  status,
	}, nil
}

// RunDockerCommand executes a docker command and returns its stdout.
func RunDockerCommand(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %s (%w)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// WaitForPort waits until the given host:port is reachable or the timeout expires.
func WaitForPort(host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %s after %v", addr, timeout)
}

// containerLogs returns the last N lines of a container's logs.
type dockerInspectJSON struct {
	State struct {
		Status string `json:"Status"`
	} `json:"State"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

// InspectContainerFull returns parsed docker inspect JSON for a container.
func InspectContainerFull(name string) (*dockerInspectJSON, error) {
	out, err := RunDockerCommand("inspect", name)
	if err != nil {
		return nil, err
	}
	var results []dockerInspectJSON
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		return nil, fmt.Errorf("parse docker inspect: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no container found: %s", name)
	}
	return &results[0], nil
}
