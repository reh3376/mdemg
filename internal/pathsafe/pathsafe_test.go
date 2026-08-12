package pathsafe

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSafeJoinUnderDir_HappyPath — the canonical valid case (a simple
// subdirectory name resolves to a child of baseDir).
func TestSafeJoinUnderDir_HappyPath(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := SafeJoinUnderDir(base, "child")
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	// Canonicalize the expected side too (macOS /var vs /private/var).
	wantAbs, _ := filepath.Abs(filepath.Join(base, "child"))
	if resolved, err := filepath.EvalSymlinks(wantAbs); err == nil {
		wantAbs = resolved
	}
	if got != wantAbs {
		t.Errorf("got %q want %q", got, wantAbs)
	}
}

// TestSafeJoinUnderDir_RejectsDotDot — a `..` segment in the untrusted
// input must return ErrTraversal.
func TestSafeJoinUnderDir_RejectsDotDot(t *testing.T) {
	base := t.TempDir()
	cases := []string{
		"../etc/passwd",
		"foo/../../etc/passwd",
		"..",
		"foo/..",
	}
	for _, in := range cases {
		if _, err := SafeJoinUnderDir(base, in); !errors.Is(err, ErrTraversal) {
			t.Errorf("SafeJoinUnderDir(%q) err=%v, want ErrTraversal", in, err)
		}
	}
}

// TestSafeJoinUnderDir_RejectsAbsolutePath — an absolute untrusted
// input must return ErrEscape (filepath.Join would otherwise resolve
// to the absolute path itself, bypassing baseDir entirely).
func TestSafeJoinUnderDir_RejectsAbsolutePath(t *testing.T) {
	base := t.TempDir()
	if _, err := SafeJoinUnderDir(base, "/etc/passwd"); !errors.Is(err, ErrEscape) {
		t.Errorf("expected ErrEscape for absolute input, got %v", err)
	}
}

// TestSafeJoinUnderDir_RejectsSymlinkEscape — a symlink inside baseDir
// pointing OUTSIDE baseDir must be caught by the EvalSymlinks
// canonicalization + containment check.
func TestSafeJoinUnderDir_RejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	// Create a symlink at base/escape -> outside.
	linkPath := filepath.Join(base, "escape")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}
	_, err := SafeJoinUnderDir(base, "escape")
	if !errors.Is(err, ErrEscape) {
		t.Errorf("expected ErrEscape for symlink pointing outside base, got %v", err)
	}
}

// TestSafeJoinUnderDir_EmptyBase — a zero-value base is a programmer
// error, not a caller error. Loudly return ErrEmptyBase rather than
// silently joining to "" (which resolves to the process cwd).
func TestSafeJoinUnderDir_EmptyBase(t *testing.T) {
	if _, err := SafeJoinUnderDir("", "child"); !errors.Is(err, ErrEmptyBase) {
		t.Errorf("expected ErrEmptyBase, got %v", err)
	}
}

// TestSafeJoinUnderDir_ContainmentBoundary — a prefix that shares a
// path segment prefix but is NOT actually inside baseDir must be
// rejected (e.g. base=/foo/b, untrusted resolves to /foo/bar).
func TestSafeJoinUnderDir_ContainmentBoundary(t *testing.T) {
	// Sibling directories at the same level, one named as a prefix of
	// the other, verify the strict-prefix + separator check.
	parent := t.TempDir()
	baseA := filepath.Join(parent, "b")
	baseAB := filepath.Join(parent, "bar")
	if err := os.MkdirAll(baseA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(baseAB, 0o755); err != nil {
		t.Fatal(err)
	}
	// SafeJoinUnderDir(baseA, "ar") joins to /parent/b/ar (does not
	// exist, but SafeJoinUnderDir does not require existence). It
	// must NOT be misclassified as inside baseAB. Assert the joined
	// path resolves under baseA (canonicalized) and NOT under baseAB.
	got, err := SafeJoinUnderDir(baseA, "ar")
	if err != nil {
		t.Fatalf("valid subpath rejected: %v", err)
	}
	canonA := canonicalize(baseA)
	canonAB := canonicalize(baseAB)
	if !strings.HasPrefix(got+string(os.PathSeparator), canonA+string(os.PathSeparator)) {
		t.Errorf("got %q not under baseA %q", got, canonA)
	}
	if strings.HasPrefix(got+string(os.PathSeparator), canonAB+string(os.PathSeparator)) {
		t.Errorf("got %q was misclassified as under sibling baseAB %q", got, canonAB)
	}
}
