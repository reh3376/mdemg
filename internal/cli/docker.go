package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
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

// ContainerNameForProject returns a project-scoped Neo4j container name.
// The name is derived from the sanitized basename of the project directory.
func ContainerNameForProject(projectDir string) string {
	return "mdemg-neo4j-" + sanitizeSlug(filepath.Base(projectDir))
}

// VolumeNameForProject returns a project-scoped Neo4j volume name.
func VolumeNameForProject(projectDir string) string {
	return "mdemg-neo4j-data-" + sanitizeSlug(filepath.Base(projectDir))
}

// sanitizeSlug creates a DNS-safe slug from a name: lowercase, non-alphanumeric
// characters replaced with hyphens, leading/trailing hyphens trimmed, max 48 chars.
func sanitizeSlug(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else {
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	// Collapse consecutive hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	if len(slug) > 48 {
		slug = slug[:48]
		slug = strings.TrimRight(slug, "-")
	}
	if slug == "" {
		slug = "default"
	}
	return slug
}

// FindFreePort tries the preferred port first, then scans rangeStart..rangeEnd.
// Returns the first available port or an error if none found.
func FindFreePort(preferred, rangeStart, rangeEnd int) (int, error) {
	// Try preferred port first
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", preferred))
	if err == nil {
		_ = ln.Close()
		return preferred, nil
	}

	// Scan range
	for port := rangeStart; port <= rangeEnd; port++ {
		if port == preferred {
			continue // already tried
		}
		ln, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port in range %d-%d", rangeStart, rangeEnd)
}

// ReadContainerPorts extracts the host-mapped bolt and HTTP ports from a running
// Docker container using docker inspect.
func ReadContainerPorts(containerName string) (boltPort, httpPort int) {
	info, err := InspectContainerFull(containerName)
	if err != nil {
		return 0, 0
	}
	boltPort = extractHostPort(info, "7687/tcp")
	httpPort = extractHostPort(info, "7474/tcp")
	return boltPort, httpPort
}

// extractHostPort extracts the mapped host port for a given container port key.
func extractHostPort(info *dockerInspectJSON, containerPort string) int {
	if info == nil {
		return 0
	}
	bindings, ok := info.NetworkSettings.Ports[containerPort]
	if !ok || len(bindings) == 0 {
		return 0
	}
	port := 0
	for _, c := range bindings[0].HostPort {
		if c >= '0' && c <= '9' {
			port = port*10 + int(c-'0')
		}
	}
	return port
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
