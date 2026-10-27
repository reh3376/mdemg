# Tutorial Serve — Embedded Onboarding Deck at `/ui/tutorial/`

**Status**: SHIPPED
**Lineage**: ONBOARDING-TUTORIAL-SERVE-001 (2026-10-26 initial serve) → ONBOARDING-TUTORIAL-ENDUSER-001 (2026-10-26 reveal.js pivot) → tutorial-forge framework extraction (2026-10-27) → persistent TOC + Graphs sidebar (2026-10-27)
**Last verified**: 2026-10-27

> The MDEMG onboarding deck is served by the mdemg API server at
> `/ui/tutorial/`. Operators access it via the `mdemg tutorial` CLI, a
> link in the Grafana overview dashboard, or by opening the URL directly.
> No external tooling required at operator time.

---

## Why

The onboarding deck lives in `docs/onboarding/mdemg-onboarding-deck.md`.
Rendering it only in markdown would:

- Force every new engineer to install an external reader before they
  can see the walkthrough.
- Leave beta testers operating a running MDEMG instance with no path
  to the tutorial at all.
- Lose section-level navigation and TOC-safe anchors when the deck is
  shared as flat GitHub markdown.

Serving the deck as pre-rendered HTML embedded in the mdemg binary
closes the gap. Zero external dependencies at operator time; the
tutorial is a feature of the running system, not a prerequisite.

---

## Choices

| Choice | Picked | Rejected | Why |
|---|---|---|---|
| **Output shape** | Plain HTML/CSS long-form document | reveal.js slide deck | A 231-section engineering walkthrough is a document, not a slideshow. The reveal.js scroll-view fought the content model; the pivot (2026-10-26) made every layout concern go away. |
| **Build tool** | `scripts/tutorial-forge/` framework (Node + marked + headless Puppeteer) | reveal-md + `scripts/tutorial/` | After the pivot the pipeline had enough reusable shape (acronym expansion, Mermaid pre-render, TOC) that extraction into a reusable framework was warranted. The forge is repo-agnostic; mdemg is now its reference consumer. |
| **Mermaid** | Pre-rendered to SVG at build time via headless Puppeteer | Client-side runtime (ship `mermaid.min.js`) | Dropped ~2.5 MB of runtime; every diagram gets the shared interactive `.svg-figure` viewer uniformly. |
| **TOC** | Persistent fixed left-margin sidebar with Section + Graphs sub-sections, scroll-sync via IntersectionObserver | Nothing / inline TOC | A ~230-section deck without persistent nav is a scroll hunt. The sidebar is a load-bearing framework contract (FSD §15). |
| **Delivery** | `//go:embed all:staged` in `internal/cli/tutorial_templates/` | On-disk materialize at install time | No filesystem I/O at serve time; `http.FS` reads directly from the embed. |
| **Drift protection** | `make verify-tutorial-embed` SHA-breadcrumb CI gate (config-aware) | Trust the committer | Deck/CSS/viewer edits without `make sync-tutorial-embed` would ship stale HTML silently. |
| **URL path** | `/ui/tutorial/` under the existing `/ui/` mount | A new top-level route | Keeps the operator-facing UI surface coherent. |
| **Enable/disable** | `TUTORIAL_ENABLED` env var, default `true` | Always on | Operators keep an opt-out knob for exposed instances. |
| **Entry points** | `mdemg tutorial` CLI + Grafana dashboard link + direct URL | CLI only | Operators who live in Grafana shouldn't context-switch to reach the deck. |
| **Acronym expansion** | First-N occurrences get `"Expansion (ACRONYM)"`; idempotent | Readers derive meaning from context | First-use definition reduces the mental model needed to read the deck cold. |

---

## How It Works

### Build-time pipeline (`make sync-tutorial-embed`)

```
docs/onboarding/mdemg-onboarding-deck.md    (source)
docs/onboarding/slides-overrides.css        (CSS override)
docs/onboarding/svg-viewer.js               (viewer override)
docs/onboarding/assets/svg/*.svg            (hand-authored diagrams)
          │
          ▼
tutorial.config.json                        (consumer config)
          │
          ▼
scripts/tutorial-forge/cli.mjs              (Node + marked + headless Puppeteer)
  ├── transforms/acronyms.mjs               (idempotent first-N expansion)
  ├── renderers/mermaid.mjs                 (Puppeteer + mermaid.min.js → SVG)
  └── build.mjs                             (orchestrator; emits TOC + Graphs sidebar)
          │
          ▼
internal/cli/tutorial_templates/staged/
  ├── index.html                            ← one scrollable long-form document
  ├── favicon.ico
  ├── SOURCE_SHA256.txt                     ← breadcrumb for drift detection
  └── assets/
      ├── css/
      │   ├── slides-overrides.css
      │   └── zenburn.min.css               ← highlight.js code theme
      ├── js/
      │   └── svg-viewer.js                 ← interactive viewer
      └── svg/
          ├── mermaid-01.svg … mermaid-22.svg   (auto-rendered)
          └── u4-how-jiminy-thinks.svg          (hand-authored)
```

Current embed: **30 files / ~1.00 MB** (2026-10-27 verified). The
previous reveal.js-era embed was 71 files / 7.73 MB; the pre-rendered
mermaid + runtime removal dropped roughly 2.5 MB; the new TOC + rendered
SVG assets added back ~0.3 MB.

### Embed package (`internal/cli/tutorial_templates/`)

```go
//go:embed all:staged
var staged embed.FS

func FS() (fs.FS, error) {
    sub, err := fs.Sub(staged, "staged")
    // ... error if index.html missing
    return sub, nil
}
```

Served via `http.FS(fs)` — no disk materialization at runtime.

### Handler (`internal/api/handlers_tutorial.go`)

```go
// Mounted BEFORE /ui/ so longest-prefix routing resolves tutorial first.
mux.Handle("/ui/tutorial/", http.StripPrefix("/ui/tutorial/",
    http.FileServer(http.FS(tutorialFS))))
```

- Gated by `TUTORIAL_ENABLED` env. 503 with a fix-hint when disabled OR
  the embed is missing.
- Defense-in-depth `..` traversal rejection (though `http.FileServer`
  also rejects).
- One-line boot log summary names the embed file count + total bytes.

### Operator-facing shape

Each page gets:

- A **persistent left-margin sidebar** at viewport ≥980px wide:
  - **Sections** sub-section listing every H1 heading (configurable
    range).
  - **Graphs** sub-section listing every rendered diagram as a jump
    link, labeled by nearest preceding heading.
  - Scroll-sync highlights the current section as the reader scrolls.
- The main content column centered within the remaining viewport,
  max-width 1400px.
- Interactive `.svg-figure` viewer around every diagram: toolbar
  `[+] [−] [⟳] [⤢]`, wheel-zoom at cursor, drag-pan, fullscreen.
- Collapsible speaker-note blocks under section content (`note:` → `<details>`).
- Code blocks with zenburn syntax highlighting.
- First-use acronym expansions (LLM, RRF, HITL, …) so the reader
  doesn't need a glossary.

### Entry points

**1. Direct URL**
```
http://localhost:9999/ui/tutorial/
```

**2. CLI**
```bash
mdemg tutorial                 # opens the URL in the default browser
mdemg tutorial --url           # prints the URL (useful for scripts)
mdemg tutorial --check         # checks the endpoint reports 200
```

**3. Grafana**

The `mdemg-overview` dashboard has a header link "MDEMG Onboarding
Tutorial" (icon: info) that opens the deck in a new tab. Operators
living in Grafana reach the deck in one click.

---

## How to use

### For operators (reading the deck)

1. Open `http://localhost:9999/ui/tutorial/` (or any entry-point above).
2. Use the left sidebar to jump between sections or diagrams.
3. Scroll; the sidebar's current entry highlights as you move.
4. Click any `.svg-figure` diagram to zoom in; drag to pan; `[⤢]` for
   fullscreen.
5. Expand speaker-note `<details>` where present for extra context.

### For authors (editing the deck)

1. Edit `docs/onboarding/mdemg-onboarding-deck.md` with any markdown
   editor.
2. Add ```` ```mermaid ```` fences for diagrams — they render as SVG
   at build time with the interactive viewer.
3. Pin specific anchors with `<!-- .slide: id="my-anchor" -->` before
   a heading when a TOC link depends on it.
4. Add `note:` on its own line to start a speaker-note block.
5. Re-sync the embed:
   ```bash
   make sync-tutorial-embed       # runs the forge
   make verify-tutorial-embed     # confirms the drift gate is clean
   ```
6. Rebuild + restart the server to see the change at `/ui/tutorial/`.

### For integrators (reusing the framework)

The build pipeline lives at `scripts/tutorial-forge/`. See its own
documentation:

- [`scripts/tutorial-forge/README.md`](../../scripts/tutorial-forge/README.md) — operator quick-start.
- [`scripts/tutorial-forge/FSD.md`](../../scripts/tutorial-forge/FSD.md) — AI-agent functional specification.
- [`scripts/tutorial-forge/example/`](../../scripts/tutorial-forge/example/) — zero-override demo that any new repo can start from.

Mdemg's own config (`tutorial.config.json` at repo root) is the
canonical reference consumer:

```json
{
  "deck": "docs/onboarding/mdemg-onboarding-deck.md",
  "output": "internal/cli/tutorial_templates/staged",
  "title": "MDEMG Tutorial",
  "cssPath": "docs/onboarding/slides-overrides.css",
  "svgViewerPath": "docs/onboarding/svg-viewer.js",
  "assetsDir": "docs/onboarding/assets",
  "acronyms": {
    "expandCount": 2,
    "extra": { "CMS": "Cognitive Memory Substrate", ... },
    "disabled": ["MDEMG", "JIMINY", "RSIC", "J17", "MLX"]
  },
  "toc": { "enabled": true, "minLevel": 1, "maxLevel": 1, "label": "Contents" }
}
```

---

## Rollback

Three independent levers:

- `TUTORIAL_ENABLED=false` + restart the server → `/ui/tutorial/` returns
  503 with a fix-hint while every other UI surface stays up.
- Remove the Grafana link via `deploy/docker/grafana/dashboards/mdemg-overview.json`
  → operators lose the Grafana entry point but CLI + direct URL still
  work.
- Revert the sprint commits → full tombstone of the serving stack.

Each lever is reversible by its opposite.

---

## Known limitations

- **Narrow viewports hide the TOC.** Below 980px the sidebar collapses
  via media query. A collapsible/toggleable mobile variant is a
  reasonable future enhancement; not shipped v0.1.
- **The claude-in-chrome MCP has an intermittent gzip-corruption class
  on `/ui/tutorial/`** — the live-page HTML can arrive as `<pre>`-wrapped
  raw bytes in the extension's DOM read. The server-side response is
  verified correct via `curl --compressed`. Real browsers render fine.
- **231-slide deck without lazy-loading.** First paint fetches the full
  248 KB index.html; this is well below the one-trip-per-page bar and
  not currently a visible slowness.
