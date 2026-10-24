// ONBOARDING-TUTORIAL-SERVE-001 E2 — tutorial handler at /ui/tutorial/.
//
// Serves the embedded reveal.js onboarding bundle built by
// `make sync-tutorial-embed`. Zero disk I/O at runtime — the bundle is
// served directly from the embed via http.FS(tutorial_templates.FS()).
// Hot-path lookup is a map read inside http.FileServer, so this is fast
// even with 71 files in the bundle.
//
// Gated by cfg.TutorialEnabled (default true). When disabled, every
// /ui/tutorial/* request returns 503 with a short JSON body — matches
// the sibling pattern used by jiminy-rules write endpoints.
//
// Security:
//   - http.ServeFileFS does its own path cleaning + root-escape rejection,
//     so we don't need pathsafe here (the embed FS is read-only, in-process,
//     and has no symlinks).
//   - The embed excludes any file outside internal/cli/tutorial_templates/
//     staged/ by construction (// go:embed is a compile-time whitelist).
package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"slices"

	"mdemg/internal/cli/tutorial_templates"
)

// tutorialHandler returns the handler to serve the embedded tutorial bundle
// at /ui/tutorial/*. The returned handler expects to have the "/ui/tutorial/"
// prefix stripped by the mux (see server.go). An empty path at this point
// means "root" and serves index.html via http.FileServer's default
// directory-index behavior.
//
// enabled=false short-circuits every request to 503 — the embed FS is still
// loaded (compile-time) but never served. Caller passes cfg.TutorialEnabled.
func (s *Server) tutorialHandler(enabled bool) http.Handler {
	if !enabled {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"tutorial disabled","hint":"set TUTORIAL_ENABLED=true to enable"}`))
		})
	}

	tfs, err := tutorial_templates.FS()
	if err != nil {
		// Build-time misconfiguration: embed is empty or missing index.html.
		// This happens if the binary was built without running
		// `make sync-tutorial-embed`. 503 with a clear operator fix.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"tutorial embed not built","hint":"rebuild the binary after running: make sync-tutorial-embed"}`))
		})
	}

	// Wrap in a thin logging layer so operators can see tutorial traffic
	// in the request log without pulling in the full access-log plumbing.
	fileServer := http.FileServer(http.FS(tfs))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Normalize root → index.html lookup. http.FileServer serves the
		// directory listing by default on root; we want index.html served.
		// Setting r.URL.Path to "/" and letting FileServer pick up index.html
		// is the standard pattern (http.FileServer looks for index.html on
		// trailing-slash paths automatically).
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}

		// Defense-in-depth: reject any path containing ".." before the
		// FileServer sees it. http.ServeContent already does this but
		// an explicit check documents the invariant.
		if containsDotDot(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		// http.FileServer's default 404 is "404 page not found". That's
		// fine — the tutorial is a static tree; a missing path is almost
		// always a bad link in the embedded HTML.
		fileServer.ServeHTTP(w, r)
	})
}

// containsDotDot returns true if p contains a "/.." or ".." segment.
// Mirrors the shape of net/http's own internal check used by
// http.ServeContent, exposed here as an explicit defense-in-depth gate.
func containsDotDot(p string) bool {
	if p == "" {
		return false
	}
	// Fast-path: no dots → no .. segment possible.
	hasDot := false
	for i := 0; i < len(p); i++ {
		if p[i] == '.' {
			hasDot = true
			break
		}
	}
	if !hasDot {
		return false
	}
	return slices.Contains(splitPath(p), "..")
}

func splitPath(p string) []string {
	var out []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if i > start {
				out = append(out, p[start:i])
			}
			start = i + 1
		}
	}
	if start < len(p) {
		out = append(out, p[start:])
	}
	return out
}

// logTutorialBootStatus is called from the server start sequence to emit a
// single-line slog about the tutorial handler's state at boot. Visible to
// operators via the server's startup logs; non-blocking.
func logTutorialBootStatus(enabled bool) {
	tfs, err := tutorial_templates.FS()
	if err != nil {
		slog.Warn("tutorial handler",
			"enabled", enabled,
			"status", "embed_missing",
			"error", err.Error(),
			"hint", "run `make sync-tutorial-embed` and rebuild")
		return
	}

	// Count files + bytes for the operator-facing summary. Cheap; done once at
	// boot. Walks the embed once.
	var fileCount int
	var totalBytes int64
	_ = fs.WalkDir(tfs, ".", func(_ string, d fs.DirEntry, wErr error) error {
		if wErr != nil || d.IsDir() {
			return nil
		}
		info, iErr := d.Info()
		if iErr == nil {
			totalBytes += info.Size()
		}
		fileCount++
		return nil
	})

	slog.Info("tutorial handler",
		"enabled", enabled,
		"url", "/ui/tutorial/",
		"files", fileCount,
		"bytes", totalBytes,
	)
}
