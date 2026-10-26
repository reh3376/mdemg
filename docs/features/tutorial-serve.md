# Tutorial Serve — Embedded Onboarding Deck at `/ui/tutorial/`

**Status**: SHIPPED (ONBOARDING-TUTORIAL-SERVE-001, 2026-10-26)
**Sprint**: [`docs/development/onboarding-tutorial-serve-001/`](../development/onboarding-tutorial-serve-001/)
**Last verified**: 2026-10-26

> The MDEMG onboarding deck (shipped by MDEMG-ONBOARDING-DECK-001 as an Obsidian Advanced Slides document) is now served by the mdemg API server at `/ui/tutorial/`. Operators access it via `mdemg tutorial`, a Grafana overview link, or by navigating directly in a browser. No external tooling required.

---

## Why

The onboarding deck lives in `docs/onboarding/mdemg-onboarding-deck.md` as Obsidian Advanced Slides. That's great for authoring + preview, but:

- A new engineer joining the project has to install Obsidian + the Advanced Slides plugin before they can see the walkthrough.
- A beta tester operating a running MDEMG instance has no path to the tutorial at all.
- Sharing via GitHub flat markdown loses the slide semantics (no slide navigation, no fork anchors).

Serving the deck as pre-rendered reveal.js HTML embedded in the mdemg binary closes the gap. Zero external dependencies at operator time; the tutorial is a feature of the running system, not a prerequisite.

---

## Choices

| Choice | Picked | Rejected | Why |
|---|---|---|---|
| **Export format** | Pre-built reveal.js static bundle via `reveal-md` | Runtime render (markdown-it + reveal.js served separately) | Pre-built is reproducible, SHA-pinned, zero-runtime-cost, and the binary ships the exact bytes CI tested. |
| **Delivery** | `//go:embed all:staged` in `internal/cli/tutorial_templates/` | On-disk materialize at install time | No file-system I/O at serve time; `http.FS` reads directly from the embed. |
| **Build location** | `scripts/tutorial/export.mjs` (Node + reveal-md pinned to 6.1.4) | Pure-Go reveal.js render | reveal-md is the canonical tool; reinventing it in Go would drift from the reveal.js spec. |
| **Drift protection** | `make verify-tutorial-embed` SHA-breadcrumb CI gate | Trust the committer | Deck edits without `make sync-tutorial-embed` would ship stale HTML silently. |
| **URL path** | `/ui/tutorial/` under existing `/ui/` mount | A new top-level route | Keeps the operator-facing UI surface coherent (`/ui/` = everything operators look at). |
| **Enable/disable** | `TUTORIAL_ENABLED` env, default `true` | Always on | Beta testers may want to disable public access on exposed instances; operators keep the knob. |
| **Entry points** | CLI (`mdemg tutorial`) + Grafana dashboard link | CLI only | Operators who live in Grafana shouldn't need to switch to a terminal to reach the deck. |

---

## How It Works

### Build-time pipeline (`make sync-tutorial-embed`)

```
docs/onboarding/mdemg-onboarding-deck.md
docs/onboarding/slides-overrides.css
          │
          ▼
scripts/tutorial/export.mjs    (Node + reveal-md@6.1.4 + mermaid@11.4.1 pinned)
          │
          ▼
internal/cli/tutorial_templates/staged/
  ├── index.html                     ← reveal.js entry point
  ├── dist/                          ← reveal.js runtime + 20 themes
  ├── plugin/                        ← markdown, highlight, zoom, etc.
  ├── _assets/docs/onboarding/
  │     └── slides-overrides.css     ← custom theme overrides
  ├── mermaid/dist/mermaid.min.js    ← bundled explicitly (reveal-md references but doesn't copy in --static)
  └── SOURCE_SHA256.txt              ← breadcrumb for drift detection
```

Current embed: **71 files / 7.73 MB** (2026-10-26 verified).

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

Mounted at `/ui/tutorial/` BEFORE `/ui/` so longest-prefix routing resolves correctly. Gated by `cfg.TutorialEnabled`:

- **Disabled** → 503 with JSON body naming `TUTORIAL_ENABLED`.
- **Embed missing** (binary built without `make sync-tutorial-embed`) → 503 with operator-fix hint.
- **Enabled** → `http.FileServer(http.FS(fs))` with defense-in-depth `..` rejection.

Boot log emits one line summarizing enabled state + file count + bytes.

### Drift gate (`make verify-tutorial-embed`)

CI runs `scripts/verify_tutorial_embed.py` which SHA-hashes the current `docs/onboarding/mdemg-onboarding-deck.md` + `docs/onboarding/slides-overrides.css` and compares against `staged/SOURCE_SHA256.txt`. If they diverge, merge is blocked with:

```
DRIFT: run 'make sync-tutorial-embed' and commit the result.
```

This mirrors the GRAFANA-SHIP (RELEASE-HYGIENE-001 E2) drift pattern.

### Entry points

1. **CLI** — `mdemg tutorial`:
   ```
   mdemg tutorial           # opens default browser against /ui/tutorial/
   mdemg tutorial --url     # prints the URL and exits
   mdemg tutorial --check   # probes the handler, exits non-zero on failure
   ```
   Endpoint resolved via the shipped priority chain (`MDEMG_ENDPOINT` env > `.mdemg.port` > `LISTEN_ADDR` > default `http://localhost:9999`).

2. **Grafana** — `mdemg-overview` dashboard header link:
   - Title: "MDEMG Onboarding Tutorial"
   - URL: `http://localhost:9999/ui/tutorial/`
   - Opens in new tab (dashboard stays visible)
   - Operators who moved the mdemg port edit the link in Grafana UI (Dashboard settings → Links).

3. **Direct URL** — any browser against `http://<mdemg-host>:<port>/ui/tutorial/`.

### Landing fork

The deck's second slide is a landing fork offering two paths:

- **Software Dev Onboarding** — the full 107-slide engineering walkthrough (the default; just press ↓/Space).
- **End User** — an end-user stub (jump via anchor link); real content lands in sibling sprint `ONBOARDING-TUTORIAL-ENDUSER-001`.

---

## How to Use

### For operators (view the tutorial)

```bash
mdemg tutorial                     # opens in default browser
mdemg tutorial --url               # just print the URL
mdemg tutorial --check             # script-friendly reachability probe
```

Or click **MDEMG Onboarding Tutorial** in the Grafana overview dashboard header.

### For operators (disable the tutorial)

Set `TUTORIAL_ENABLED=false` in `.env` and restart. The handler responds 503 with an actionable hint.

### For developers (edit the deck)

1. Edit `docs/onboarding/mdemg-onboarding-deck.md` (or `slides-overrides.css`).
2. Run `make sync-tutorial-embed` — regenerates `internal/cli/tutorial_templates/staged/`.
3. Commit both the source edit AND the resynced embed in the same commit.
4. CI's `make verify-tutorial-embed` enforces this; a source-only commit without the resync blocks merge.

**Why both must land together**: `//go:embed` is a build-time snapshot. If the deck edit lands without the resynced embed, the running binary serves the OLD HTML — operator sees content that doesn't match the markdown. The CI drift gate catches this at merge time.

---

## Related

- Deck source + authoring: [`docs/onboarding/README.md`](../onboarding/README.md), [`docs/onboarding/mdemg-onboarding-deck.md`](../onboarding/mdemg-onboarding-deck.md)
- Governance of the deck itself: [`docs/features/mdemg-onboarding-deck.md`](./mdemg-onboarding-deck.md)
- Sibling sprint (end-user path content): `ONBOARDING-TUTORIAL-ENDUSER-001` (planned)
- GRAFANA-SHIP drift pattern (precedent for `make sync-tutorial-embed` / `make verify-tutorial-embed`): [`docs/development/release-hygiene-001/`](../development/release-hygiene-001/)
