package cli

import (
	"net"
	"testing"
)

// --- ContainerNameForProject tests ---

func TestContainerNameForProject_Simple(t *testing.T) {
	got := ContainerNameForProject("/home/user/my-project")
	want := "mdemg-neo4j-my-project"
	if got != want {
		t.Errorf("ContainerNameForProject = %q, want %q", got, want)
	}
}

func TestContainerNameForProject_Uppercase(t *testing.T) {
	got := ContainerNameForProject("/home/user/MyProject")
	want := "mdemg-neo4j-myproject"
	if got != want {
		t.Errorf("ContainerNameForProject = %q, want %q", got, want)
	}
}

func TestContainerNameForProject_SpecialChars(t *testing.T) {
	got := ContainerNameForProject("/home/user/my_project.v2")
	want := "mdemg-neo4j-my-project-v2"
	if got != want {
		t.Errorf("ContainerNameForProject = %q, want %q", got, want)
	}
}

func TestContainerNameForProject_Dots(t *testing.T) {
	got := ContainerNameForProject("/home/user/com.example.project")
	want := "mdemg-neo4j-com-example-project"
	if got != want {
		t.Errorf("ContainerNameForProject = %q, want %q", got, want)
	}
}

// --- VolumeNameForProject tests ---

func TestVolumeNameForProject_Simple(t *testing.T) {
	got := VolumeNameForProject("/home/user/my-project")
	want := "mdemg-neo4j-data-my-project"
	if got != want {
		t.Errorf("VolumeNameForProject = %q, want %q", got, want)
	}
}

func TestVolumeNameForProject_MatchesContainer(t *testing.T) {
	dir := "/home/user/test-app"
	container := ContainerNameForProject(dir)
	volume := VolumeNameForProject(dir)
	// Both should use the same slug
	if container != "mdemg-neo4j-test-app" {
		t.Errorf("container = %q", container)
	}
	if volume != "mdemg-neo4j-data-test-app" {
		t.Errorf("volume = %q", volume)
	}
}

// --- sanitizeSlug tests ---

func TestSanitizeSlug_Lowercase(t *testing.T) {
	got := sanitizeSlug("MyProject")
	if got != "myproject" {
		t.Errorf("sanitizeSlug = %q, want %q", got, "myproject")
	}
}

func TestSanitizeSlug_SpecialChars(t *testing.T) {
	got := sanitizeSlug("my_project.v2@beta")
	if got != "my-project-v2-beta" {
		t.Errorf("sanitizeSlug = %q, want %q", got, "my-project-v2-beta")
	}
}

func TestSanitizeSlug_ConsecutiveHyphens(t *testing.T) {
	got := sanitizeSlug("a___b...c")
	if got != "a-b-c" {
		t.Errorf("sanitizeSlug = %q, want %q", got, "a-b-c")
	}
}

func TestSanitizeSlug_LeadingTrailing(t *testing.T) {
	got := sanitizeSlug("--my-project--")
	if got != "my-project" {
		t.Errorf("sanitizeSlug = %q, want %q", got, "my-project")
	}
}

func TestSanitizeSlug_LongName(t *testing.T) {
	long := "this-is-a-very-long-project-name-that-exceeds-forty-eight-characters-limit"
	got := sanitizeSlug(long)
	if len(got) > 48 {
		t.Errorf("sanitizeSlug length = %d, want <= 48", len(got))
	}
	// Should not end with a hyphen
	if got[len(got)-1] == '-' {
		t.Error("sanitizeSlug should not end with hyphen")
	}
}

func TestSanitizeSlug_Empty(t *testing.T) {
	got := sanitizeSlug("")
	if got != "default" {
		t.Errorf("sanitizeSlug = %q, want %q", got, "default")
	}
}

func TestSanitizeSlug_AllSpecialChars(t *testing.T) {
	got := sanitizeSlug("@#$%^&*()")
	if got != "default" {
		t.Errorf("sanitizeSlug = %q, want %q", got, "default")
	}
}

// --- FindFreePort tests ---

func TestFindFreePort_PreferredAvailable(t *testing.T) {
	// Find a free port first to use as the preferred port
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skipf("cannot listen on ephemeral port: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	freePort := addr.Port
	ln.Close()

	got, err := FindFreePort(freePort, freePort, freePort+100)
	if err != nil {
		t.Fatalf("FindFreePort error: %v", err)
	}
	if got != freePort {
		t.Errorf("FindFreePort = %d, want preferred port %d", got, freePort)
	}
}

func TestFindFreePort_PreferredBusy(t *testing.T) {
	// Occupy the preferred port
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer ln.Close()
	busyPort := ln.Addr().(*net.TCPAddr).Port

	// FindFreePort should find a different port in the range
	got, err := FindFreePort(busyPort, busyPort, busyPort+10)
	if err != nil {
		t.Fatalf("FindFreePort error: %v", err)
	}
	if got == busyPort {
		t.Errorf("FindFreePort returned busy port %d", busyPort)
	}
}
