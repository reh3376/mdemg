// ONBOARDING-TUTORIAL-SERVE-001 — pin tests for the tutorial embed.
//
// Pivoted 2026-10-26 to a plain HTML/CSS long-form document (no reveal.js).
// Verifies the embed is well-formed + exposes the invariants the handler
// and CI drift gate depend on against the new package layout:
//
//   staged/
//     index.html
//     favicon.ico
//     SOURCE_SHA256.txt
//     assets/css/slides-overrides.css
//     assets/css/zenburn.min.css
//     assets/js/mermaid.min.js
//     assets/js/svg-viewer.js
//     assets/svg/<hand-authored diagrams>
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

// TestFS_IndexHTMLShape verifies the index.html came out of the new
// marked-based pipeline. The old reveal-md output had `<div class="reveal">`;
// the new output has `<main class="deck">`. Pinning this catches a build
// regression that silently reverted to the reveal.js shell.
func TestFS_IndexHTMLShape(t *testing.T) {
	f, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	data, err := fs.ReadFile(f, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	body := string(data)
	// New-pipeline markers.
	for _, needle := range []string{
		`<main class="deck">`,
		`assets/js/mermaid.min.js`,
		`assets/js/svg-viewer.js`,
		`assets/css/slides-overrides.css`,
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("index.html missing expected marker %q — new HTML pipeline likely regressed", needle)
		}
	}
	// Old-pipeline markers that MUST NOT reappear.
	for _, forbidden := range []string{
		`<div class="reveal">`,
		`dist/reveal.js`,
		`revealOptions`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("index.html contains reveal.js-era marker %q — pivot regressed", forbidden)
		}
	}
}

// TestFS_BundlesMermaid verifies the mermaid.min.js shim the export script
// adds for offline inline rendering.
func TestFS_BundlesMermaid(t *testing.T) {
	f, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	info, err := fs.Stat(f, "assets/js/mermaid.min.js")
	if err != nil {
		t.Fatalf("mermaid.min.js not bundled at assets/js/mermaid.min.js: %v", err)
	}
	// Mermaid 11.4.x is ~2.5 MB. Sanity floor at 1 MB catches a stub.
	if info.Size() < 1024*1024 {
		t.Fatalf("mermaid.min.js suspiciously small (%d bytes) — expected ~2.5 MB", info.Size())
	}
}

// TestFS_SlideOverrideCSSPresent verifies the deck's CSS override lands at
// the new package path (`assets/css/slides-overrides.css`).
func TestFS_SlideOverrideCSSPresent(t *testing.T) {
	f, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	path := "assets/css/slides-overrides.css"
	info, err := fs.Stat(f, path)
	if err != nil {
		t.Fatalf("slides-overrides.css not in embed at %s: %v", path, err)
	}
	if info.Size() < 500 {
		t.Fatalf("slides-overrides.css suspiciously small (%d bytes) — stub emission", info.Size())
	}
}

// TestFS_SVGViewerPresent verifies the interactive SVG viewer JS lands in
// the embed. Without it, the figure containers lose their zoom/pan controls.
func TestFS_SVGViewerPresent(t *testing.T) {
	f, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	info, err := fs.Stat(f, "assets/js/svg-viewer.js")
	if err != nil {
		t.Fatalf("svg-viewer.js not in embed: %v", err)
	}
	if info.Size() < 500 {
		t.Fatalf("svg-viewer.js suspiciously small (%d bytes) — stub emission", info.Size())
	}
}

// TestFS_NoAbsolutePathLeaks defends the reproducibility invariant: absolute
// paths in index.html would make the embed byte-differ across machines.
// Pivot kept the "relative paths only" contract.
func TestFS_NoAbsolutePathLeaks(t *testing.T) {
	f, err := FS()
	if err != nil {
		t.Fatalf("FS() error: %v", err)
	}
	data, err := fs.ReadFile(f, "index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	for _, needle := range []string{"/Users/", "/home/", "/private/tmp/", `C:\`} {
		if strings.Contains(string(data), needle) {
			t.Errorf("index.html contains absolute-path leak %q — export script must use relative paths", needle)
		}
	}
}

// TestManifest_FilesPresent asserts the embed has the small, well-defined
// set of files the new HTML pipeline emits. Floor at 6 to catch a stub
// emission; ceiling at 200 to catch an accidental over-copy that would
// drag the bundle size back up.
func TestManifest_FilesPresent(t *testing.T) {
	files, err := Manifest()
	if err != nil {
		t.Fatalf("Manifest() error: %v", err)
	}
	if len(files) < 6 {
		t.Fatalf("embed suspiciously small (%d files) — expected index.html + favicon.ico + SOURCE_SHA256.txt + 4+ asset files", len(files))
	}
	if len(files) > 200 {
		t.Fatalf("embed suspiciously large (%d files) — pivot regression; the new pipeline should emit ~8-30 files total", len(files))
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

// TestEmbedSize_Sane: new-pipeline bundle is ~2.7 MB (deck HTML + mermaid.min.js
// + CSS + highlight style + svg-viewer.js + hand-authored SVGs). The floor at
// 1.5 MB catches a stub emission; ceiling at 15 MB catches a regression that
// resurrects the reveal.js runtime (~7 MB) or over-copies node_modules.
func TestEmbedSize_Sane(t *testing.T) {
	total, err := EmbedSize()
	if err != nil {
		t.Fatalf("EmbedSize() error: %v", err)
	}
	if total < 1_500_000 || total > 15*1024*1024 {
		t.Errorf("embed size %d bytes outside expected 1.5-15 MB range — investigate", total)
	}
}
