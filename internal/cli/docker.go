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

const (
	minDockerMemoryGB = 4
	minDockerCPUs     = 2
)

// DockerResources holds Docker Desktop resource allocation.
type DockerResources struct {
	MemoryBytes int64
	CPUs        int
}

// CheckDockerResources queries Docker for allocated resources and returns warnings
// if below minimums. Returns nil warnings if resources are adequate or Docker is unavailable.
func CheckDockerResources() (DockerResources, []string) {
	var res DockerResources

	out, err := RunDockerCommand("info", "--format", "{{json .}}")
	if err != nil {
		return res, nil // Docker not running — don't warn here, other checks handle it
	}

	var info struct {
		MemTotal int64 `json:"MemTotal"`
		NCPU     int   `json:"NCPU"`
	}
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return res, nil // Parse failure — don't block init
	}

	res.MemoryBytes = info.MemTotal
	res.CPUs = info.NCPU

	var warnings []string
	memGB := float64(info.MemTotal) / (1024 * 1024 * 1024)
	if memGB < float64(minDockerMemoryGB) {
		warnings = append(warnings, fmt.Sprintf(
			"Docker has %.1f GB RAM (minimum: %d GB) — increase via Docker Desktop > Settings > Resources",
			memGB, minDockerMemoryGB))
	}
	if info.NCPU < minDockerCPUs {
		warnings = append(warnings, fmt.Sprintf(
			"Docker has %d CPU(s) (minimum: %d) — increase via Docker Desktop > Settings > Resources",
			info.NCPU, minDockerCPUs))
	}

	return res, warnings
}

// Neo4jMemoryConfig holds computed Neo4j memory settings based on available RAM.
type Neo4jMemoryConfig struct {
	Pagecache   string
	HeapInit    string
	HeapMax     string
	MaxPoolSize int
}

// neo4jMemoryTier returns Neo4j memory settings appropriate for the given
// system RAM (in gigabytes). The tiers ensure Neo4j does not overwhelm
// smaller machines while still performing well on larger ones.
func neo4jMemoryTier(systemRAMGB int) Neo4jMemoryConfig {
	switch {
	case systemRAMGB <= 16:
		return Neo4jMemoryConfig{"512M", "256M", "512M", 50}
	case systemRAMGB <= 32:
		return Neo4jMemoryConfig{"1G", "512M", "1G", 75}
	case systemRAMGB <= 64:
		return Neo4jMemoryConfig{"2G", "512M", "2G", 100}
	default:
		return Neo4jMemoryConfig{"2G", "1G", "2G", 100}
	}
}

// DetectSystemRAMGB returns the total system RAM in gigabytes (rounded down),
// or 0 if detection fails.
func DetectSystemRAMGB() int {
	bytes, err := detectSystemRAMBytes()
	if err != nil {
		return 0
	}
	return int(bytes / (1024 * 1024 * 1024))
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
//
// Docker Compose uses underscores to join the project name and service volume
// name, e.g. "mdemg_neo4j_data" when run from a directory called "mdemg".
// The legacy CLI path used hyphens ("mdemg-neo4j-data-mdemg").  To avoid
// silently creating a second empty volume, this function checks whether the
// docker-compose-style volume already exists and returns it when found.
func VolumeNameForProject(projectDir string) string {
	slug := sanitizeSlug(filepath.Base(projectDir))

	// Docker Compose derives its project name from the directory base name,
	// lowercased, with non-alphanumeric characters replaced by hyphens —
	// then joins it to the volume name with an underscore.
	composeVolume := slug + "_neo4j_data"
	if dockerVolumeExists(composeVolume) {
		return composeVolume
	}

	// Legacy / standalone CLI volume name (hyphen-separated).
	return "mdemg-neo4j-data-" + slug
}

// dockerVolumeExists reports whether a Docker volume with the given name
// exists on the local daemon.  Returns false on any error (Docker not
// available, volume not found, etc.) so callers can fall back gracefully.
func dockerVolumeExists(name string) bool {
	out, err := RunDockerCommand("volume", "inspect", "--format", "{{.Name}}", name)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == name
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

// tryMigrateVolume checks for legacy hyphen-named volumes and warns about
// the naming mismatch between `mdemg db start` (hyphens) and `docker compose`
// (underscores). If the legacy volume has data and the compose volume doesn't
// exist, it offers a migration command. If both exist, it warns about duplicate
// data. Returns the recommended volume name to use.
func tryMigrateVolume(projectDir string) string {
	slug := sanitizeSlug(filepath.Base(projectDir))
	composeVol := slug + "_neo4j_data"
	legacyVol := "mdemg-neo4j-data-" + slug

	composeExists := dockerVolumeExists(composeVol)
	legacyExists := dockerVolumeExists(legacyVol)

	switch {
	case composeExists && legacyExists:
		fmt.Printf("WARNING: Both volumes exist — data may be split:\n")
		fmt.Printf("  Compose volume: %s (preferred)\n", composeVol)
		fmt.Printf("  Legacy volume:  %s\n", legacyVol)
		fmt.Printf("  To consolidate, stop all containers, then:\n")
		fmt.Printf("    docker run --rm -v %s:/src -v %s:/dst alpine sh -c 'cp -a /src/. /dst/'\n", legacyVol, composeVol)
		fmt.Printf("    docker volume rm %s\n", legacyVol)
		return composeVol
	case legacyExists && !composeExists:
		fmt.Printf("NOTE: Migrating to compose-compatible volume name:\n")
		fmt.Printf("  %s → %s\n", legacyVol, composeVol)
		// Create the compose volume and copy data
		if _, err := RunDockerCommand("volume", "create", composeVol); err != nil {
			fmt.Printf("  Migration failed (volume create): %v\n", err)
			fmt.Printf("  Using legacy volume: %s\n", legacyVol)
			return legacyVol
		}
		_, cpErr := RunDockerCommand("run", "--rm",
			"-v", legacyVol+":/src",
			"-v", composeVol+":/dst",
			"alpine", "sh", "-c", "cp -a /src/. /dst/")
		if cpErr != nil {
			fmt.Printf("  Migration failed (data copy): %v\n", cpErr)
			fmt.Printf("  Using legacy volume: %s\n", legacyVol)
			return legacyVol
		}
		fmt.Printf("  Data migrated. Legacy volume retained as backup.\n")
		fmt.Printf("  Remove when verified: docker volume rm %s\n", legacyVol)
		return composeVol
	default:
		return VolumeNameForProject(projectDir)
	}
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
