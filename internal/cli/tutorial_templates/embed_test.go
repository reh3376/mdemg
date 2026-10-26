// ONBOARDING-TUTORIAL-SERVE-001 — pin tests for the tutorial embed.
// Verifies the embed is well-formed + exposes the invariants the handler
// and CI drift gate depend on.
package tutorial_templates

import (
	"io/fs"
	"strings"
	"testing"
)

// TestFS_IndexHTMLPresent is the load-bearing invariant — the handler returns
// a 503 if index.html is missing. Catches a sync that silently emitted an
// empty staged/ dir.
func TestFS_IndexHTMLPresent(t *testing.T) {
	f, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	info, err := fs.Stat(f, "index.html")
	if err != nil {
		t.Fatalf("fs.Stat(index.html): %v", err)
	}
	if info.Size() < 1000 {
		t.Fatalf("index.html suspiciously small (%d bytes) — export likely emitted a shell", info.Size())
	}
}

// TestFS_ServesRevealJS verifies the reveal.js runtime assets came along.
// Without these, the embedded index.html renders a blank page.
func TestFS_ServesRevealJS(t *testing.T) {
	f, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	required := []string{
		"dist/reveal.css",
		"dist/reveal.js",
		"dist/theme/night.css",
	}
	for _, path := range required {
		info, sErr := fs.Stat(f, path)
		if sErr != nil {
			t.Errorf("required asset missing: %s (err=%v)", path, sErr)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("required asset empty: %s", path)
		}
	}
}

// TestFS_BundlesMermaid verifies the mermaid.min.js shim the export script
// adds for offline rendering (reveal-md --static references
// ./mermaid/dist/mermaid.min.js but doesn't bundle it; the export script
// copies it from scripts/tutorial/node_modules/mermaid/dist/).
func TestFS_BundlesMermaid(t *testing.T) {
	f, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	info, err := fs.Stat(f, "mermaid/dist/mermaid.min.js")
	if err != nil {
		t.Fatalf("mermaid.min.js not bundled — the export script's manual copy step didn't run: %v", err)
	}
	// Mermaid 11.4.x is ~2.5 MB. Sanity floor at 1 MB catches a stub.
	if info.Size() < 1024*1024 {
		t.Fatalf("mermaid.min.js suspiciously small (%d bytes) — expected ~2.5 MB", info.Size())
	}
}

// TestFS_SlideOverrideCSSPresent verifies the bottom-clipping fix CSS
// traveled. Without it, the exported HTML inherits reveal.js night-theme
// defaults and clips the F1.2 slide's third bullet.
func TestFS_SlideOverrideCSSPresent(t *testing.T) {
	f, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	path := "_assets/docs/onboarding/slides-overrides.css"
	info, err := fs.Stat(f, path)
	if err != nil {
		t.Fatalf("slides-overrides.css not in embed at %s: %v", path, err)
	}
	if info.Size() < 500 {
		t.Fatalf("slides-overrides.css suspiciously small (%d bytes) — stub emission", info.Size())
	}
}

// TestFS_NoAbsolutePathLeaks defends the reproducibility invariant from the
// E1 build pipeline: reveal-md bakes argv paths into index.html verbatim, so
// absolute paths would make the embed byte-differ across machines. The export
// script passes relative paths from REPO_ROOT; this test regression-locks that.
func TestFS_NoAbsolutePathLeaks(t *testing.T) {
	f, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	data, err := fs.ReadFile(f, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	// Any absolute path reference breaks reproducibility across machines.
	// Catches /Users/, /home/, C:\, and GitHub Actions' /home/runner/.
	for _, needle := range []string{"/Users/", "/home/", "/private/tmp/", `C:\`} {
		if strings.Contains(string(data), needle) {
			t.Errorf("index.html contains absolute-path leak %q — export script must use relative paths from REPO_ROOT", needle)
		}
	}
}

// TestManifest_FilesPresent asserts the embed has enough files to be a
// real reveal.js bundle (reveal.js alone ships ~40 files; mermaid + CSS
// + highlight.js + the index add more).
func TestManifest_FilesPresent(t *testing.T) {
	files, err := Manifest()
	if err != nil {
		t.Fatalf("Manifest() error: %v", err)
	}
	if len(files) < 30 {
		t.Fatalf("embed suspiciously small (%d files) — expected a full reveal.js bundle (~60-80)", len(files))
	}
}

// TestSourceSHA_Present verifies the SOURCE_SHA256.txt breadcrumb landed.
// CI drift gate relies on this file to detect "deck edited without sync."
func TestSourceSHA_Present(t *testing.T) {
	deckSHA, cssSHA, err := SourceSHA()
	if err != nil {
		t.Fatalf("SourceSHA() error: %v", err)
	}
	if len(deckSHA) != 64 || len(cssSHA) != 64 {
		t.Fatalf("expected 64-char hex SHAs, got deck=%d css=%d", len(deckSHA), len(cssSHA))
	}
}

// TestEmbedSize_Sane: 7-10 MB total is the known-good range (reveal.js
// dist + plugins + mermaid.min.js + 15 Mermaid diagrams + the deck).
// Catches a build regression that silently stripped half the embed.
func TestEmbedSize_Sane(t *testing.T) {
	total, err := EmbedSize()
	if err != nil {
		t.Fatalf("EmbedSize() error: %v", err)
	}
	if total < 5*1024*1024 || total > 15*1024*1024 {
		t.Errorf("embed size %d bytes outside expected 5-15 MB range — investigate", total)
	}
}
