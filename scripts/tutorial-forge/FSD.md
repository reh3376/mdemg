# tutorial-forge — Functional Specification (AI Agent Reference)

**Audience**: an AI coding agent that must understand, integrate, extend, or
debug this framework in any repo. This document is *the* reference. Prefer it
over the human-oriented `README.md` when deciding behavior or diagnosing a
failure.

**Document contract**: every claim here either (a) describes the shipped
behavior as observable in the files named, or (b) is explicitly marked as a
convention / non-goal. If source and FSD disagree, source wins and the FSD
is the bug — update it in the same PR.

---

## 1. Purpose

tutorial-forge is a build-time pipeline that converts **one markdown deck
file** into **one self-contained static HTML package** suitable for:

- Serving from any static host (GitHub Pages, Netlify, nginx, static CDN).
- Embedding in a Go binary via `//go:embed`.
- Attaching to a release artifact.

The pipeline applies four non-trivial transforms during the build:

| # | Transform | Why |
| --- | --- | --- |
| 1 | ```` ```mermaid ```` → SVG (headless Puppeteer + mermaid.min.js) | Ship no runtime; every diagram gets the interactive viewer uniformly. |
| 2 | First-N acronym expansion (`"LLM"` → `"Large Language Model (LLM)"`) | Defines each term on first use; idempotent across builds. |
| 3 | Heading-ID resolution (explicit pins first, then auto-slug with dedup) | Stable TOC anchors even when heading text changes. |
| 4 | Speaker-note extraction (`note: …` → collapsible `<details>`) | Separates presenter notes from reader content in-source. |

Everything else is passthrough: GFM markdown via `marked`, zenburn
highlighting for fenced code, standard markdown tables/lists/blockquotes.

## 2. Non-goals

- **Not a slide deck engine.** Output is one scrollable long-form HTML page,
  not reveal.js. The deck's `---` separators become visual rules, not
  slide boundaries. (This was a deliberate pivot — see commit `0f12fbfa`
  in the mdemg repo and the FSD's §12 Rollback for context.)
- **Not a static-site generator.** Produces exactly one page per invocation.
  Multi-page / multi-deck is out of scope.
- **Not a Mermaid dialect extender.** Uses stock mermaid.min.js 11.x. If a
  diagram doesn't render in the official playground, it won't render here.
- **Not a runtime.** The emitted bundle has only one tiny client-side JS
  file (`svg-viewer.js`, ~9 KB, zero deps). No build step at serve time.
- **Not a PR / commit author.** The framework writes files and exits; CI /
  Makefile / human / another agent owns the git flow.

## 3. Architecture

```
                       tutorial.config.json
                              │
                              ▼
       ┌──────────────────── cli.mjs ────────────────────┐
       │           (parses args, loads config)            │
       └──────────────────┬────────────┬─────────────────┘
                          │ build      │ expand-acronyms
                          ▼            ▼
                      build.mjs      transforms/acronyms.mjs
                          │
            ┌─────────────┼──────────────────────┐
            │             │                       │
            ▼             ▼                       ▼
    transforms/      renderers/              marked.parse
    acronyms.mjs     mermaid.mjs             (upstream npm pkg)
    (JS, pure)       (Puppeteer + mermaid)
            │             │                       │
            └─────────────┼───────────────────────┘
                          ▼
                 <config.output>/
                   index.html
                   favicon.ico
                   SOURCE_SHA256.txt
                   assets/
                     css/
                     js/
                     svg/
```

### 3.1 Data flow at build time

1. CLI loads JSON config, resolves paths relative to the config file's dir.
2. Deck markdown is read + preprocessed:
   - Optional YAML frontmatter stripped.
   - `./assets/` → `assets/` path rewrites.
   - Acronym expansion (idempotent, see §4.2).
3. Deck is split on `\n---\n` into chunks (one per logical section).
4. Per chunk:
   - Extract explicit `<!-- .slide: id="X" -->` directive → record pin.
   - Extract trailing `note: ...` block → wrap in `<details>`.
   - Replace ```` ```mermaid ```` fences with `@@MERMAID_PLACEHOLDER_N@@` tokens
     (sources collected into array; order preserved).
5. Chunks rejoined with `<hr class="slide-break" />`.
6. `marked.parse` → HTML.
7. Mermaid pass: Puppeteer renders each source to SVG, writes
   `<output>/assets/svg/mermaid-NN.svg`, swaps each placeholder for a
   `<figure class="svg-figure" tabindex="0"><img …/></figure>`.
8. Heading-ID pass A: attach explicit pin IDs to matching headings in
   source order (first unlocked heading whose plain-text matches wins).
9. Heading-ID pass B: auto-slug every remaining heading. Deck-wide
   uniqueness via `-2`, `-3`, … suffixes.
10. HTML shell assembled around body.
11. Aux assets copied (`assets/css/{slides-overrides,zenburn.min}.css`,
    `assets/js/svg-viewer.js`, hand-authored assets from `config.assetsDir`,
    favicon).
12. `SOURCE_SHA256.txt` written with the SHA of deck, CSS, SVG viewer.

### 3.2 Dependency graph (framework → npm)

- `marked@13.0.3` — markdown → HTML.
- `mermaid@11.16.1` — diagram runtime, read at build time by Puppeteer.
- `puppeteer@23.9.0` — headless Chromium for Mermaid render. Downloads
  bundled Chromium (~150 MB) on first `npm install`.
- `highlight.js@11.11.1` — ships `zenburn.min.css` only (code renders at
  serve time via plain `<pre><code class="language-…">` which browsers
  highlight if a code engine is loaded; the framework's bundle does not
  include highlight.js runtime — the ULTRA-light choice).
- `reveal-md@6.1.4` — kept for its bundled `favicon.ico` only. Zero runtime
  use. Can be removed if a framework consumer provides its own icon.

## 4. File reference

### 4.1 `cli.mjs` (entry point)

**Shape**: `#!/usr/bin/env node` ESM module.

**Argv parsing**: positional subcommand (`build` default when first arg
starts with `-`), then `--config|-c <path>` (default `./tutorial.config.json`),
`--write` (expand-acronyms only), `--help|-h`.

**Subcommands**:

| Subcommand | Behavior | Exit code |
| --- | --- | --- |
| `build` | Calls `build.mjs::build(config, configDir)`. Overwrites output dir. | 0 success, 1 fatal |
| `expand-acronyms` | Dry-run or (with `--write`) persists the acronym transform to the deck source file. | 0 |
| `help` / `--help` / `-h` | Prints help; no side effects. | 0 |

**Exit codes beyond 0**:
- `1` — fatal error (ENOENT, Mermaid parse failure, etc.).
- `2` — argv / config load error.

**Error stream**: all errors land on stderr prefixed `[tutorial-forge] FATAL:`.
Stack trace included.

### 4.2 `transforms/acronyms.mjs`

**Public API**:

```js
loadAcronyms(configAcronyms) → { ACRONYM: expansion, … }
expandAcronyms(deckText, acronyms, expandCount = 2) → { text, applied }
expandAcronymsInFile(deckPath, acronyms, expandCount) → { changed, text, applied, path }
```

**Idempotency contract** *(load-bearing)*: before expanding anything the
transform pre-scans the deck for existing `"${expansion} (${ACRONYM})"`
substrings and counts them toward the per-acronym budget. Running the
transform repeatedly against an already-expanded deck is a no-op. The
`applied` return value reports **only new expansions**, not pre-existing.

**Masked regions** (acronyms inside these are never matched):

- Fenced code blocks (` ``` ` → ` ``` `, tracked by line toggle).
- Lines that are pure HTML comments (`<!-- … -->`).
- Inline code spans (`` `…` ``).
- HTML tags (`<…>`).
- Markdown link URL parts (`](…)`).

**Match constraint**: `(?<!\w)(ACRONYM)(?!\w)` with longest-first alternation
so `UVTS` doesn't lose to `UV`. Match is skipped if preceded by `(` (that's
inside an already-expanded form's `(ACRONYM)` suffix).

**Config merge**:
```
merged = { ...defaults_acronyms_json, ...config.acronyms.extra }
for key in config.acronyms.disabled: delete merged[key]
```

### 4.3 `renderers/mermaid.mjs`

**Public API**:

```js
renderMermaidSources(sources: string[], opts?: { theme?: 'default'|'dark' })
  → Promise<string[]>  // SVG strings in input order
```

**Internals**:

- Launches one Puppeteer browser per invocation (`--no-sandbox`,
  `--disable-setuid-sandbox`).
- Sets page content ONCE with the bundled mermaid.min.js inlined as a
  `<script>` tag (no file:// URL required; works offline).
- `mermaid.initialize({ htmlLabels: true, flowchart.curve: 'basis', … })`.
  Theme passed through via `opts.theme` (default: `'default'` = light).
- For each source: pre-normalize (`<br/>` → `\n`; auto-quote `[labels]`
  containing `( ) / & \n`), then `page.evaluate(mermaid.render(id, src))`.
  Fails hard on parse error with the full source dumped to stderr.

**Pre-normalization rules** *(load-bearing)*: Mermaid's strict parser rejects
unquoted `(` inside rectangle `[label]` syntax because it reads as a stadium
shape marker. Framework auto-wraps any `[content]` containing special chars
in `"..."` quotes. Author-side quoting is still recommended for robustness.

**Error signature on parse failure**:
```
Error: mermaid render failed for block N: Parse error on line X:
...<cursor-anchor>...
Expecting '…', got '…'
--- source ---
<the full source>
--- end ---
```

### 4.4 `build.mjs` (orchestrator)

**Public API**:

```js
build(config: TutorialConfig, configDir: string) → Promise<{
  files: number,
  bytes: number,
  mermaidCount: number,
  acronymsApplied: Record<string, number>,
  deckSha: string,
  cssSha: string,
  viewerSha: string
}>
```

**Required config fields**: `deck`, `output`.

**Failure modes & exit** (via `fail()` → `process.exit(1)`):
- Missing deck source → `deck source not found: <abs path>`.
- Missing CSS override → `CSS not found: <abs path>`.
- Missing SVG viewer → `SVG viewer not found: <abs path>`.
- Missing framework-shipped highlight style or mermaid.min.js → corrupt
  install; run `npm install` in `scripts/tutorial-forge/`.

**Side effects**:
1. `rmSync(OUTPUT_DIR, { recursive: true, force: true })` — destructive to
   output dir. Agents MUST ensure `config.output` is not pointed at
   something a user would hate to lose. The directory is wiped on every
   build.
2. Writes files under `OUTPUT_DIR` only. Never writes to deck source or
   any other input (unless `expand-acronyms --write` was invoked).

**Mermaid rendering is skipped** when either (a) no `` ```mermaid `` blocks
present, OR (b) `config.mermaid.skipRender === true`. In case (b),
placeholders remain in the HTML as `@@MERMAID_PLACEHOLDER_N@@` text — the
build does NOT fail, so this flag is useful for dry runs that skip the
Puppeteer startup cost but WILL produce a visually broken HTML. Do not
use for production builds.

### 4.5 Framework-shipped defaults

| File | Role | Can be overridden by config |
| --- | --- | --- |
| `templates/default.css` | Dark-theme CSS, 1400px max-width centered, `.svg-figure` styling, speaker-notes, slide-break rules | Yes: `config.cssPath` |
| `templates/svg-viewer.js` | Interactive viewer: toolbar `[+][−][⟳][⤢]`, wheel-zoom at cursor, drag-pan, touch pinch, double-click reset, fullscreen; MutationObserver-safe idempotent attach | Yes: `config.svgViewerPath` |
| `defaults/acronyms.json` | ~55 industry-standard terms (AI, API, CLI, CSS, HTML, HITL, LLM, …) | Via `config.acronyms.extra` (adds) and `config.acronyms.disabled` (removes) |

Framework defaults for `highlight.js/styles/base16/zenburn.min.css`,
mermaid.min.js, and favicon.ico are read from `node_modules/`. These are
NOT user-configurable except by config.iconPath for the favicon.

## 5. Configuration schema

Full type definition (TypeScript shape; file itself is plain JSON):

```ts
interface TutorialConfig {
  // REQUIRED
  deck: string;                 // path to markdown source, relative to config file
  output: string;               // path to output directory, relative to config file

  // Shell / branding
  title?: string;               // default: 'Tutorial' — <title> element text
  iconPath?: string;            // default: framework favicon from reveal-md

  // Overrides (fall back to framework defaults when absent)
  cssPath?: string;             // default: templates/default.css
  svgViewerPath?: string;       // default: templates/svg-viewer.js

  // Hand-authored assets
  assetsDir?: string;           // optional; contents merged into <output>/assets

  // Acronym transform
  acronyms?: {
    expandCount?: number;       // default: 2
    extra?: Record<string, string>;  // merged over defaults/acronyms.json
    disabled?: string[];        // acronyms dropped from merged dict
  };

  // Mermaid
  mermaid?: {
    theme?: 'default' | 'dark'; // default: 'default' (light); .svg-figure has white BG
    skipRender?: boolean;       // default: false; see §4.4 warning
  };

  // Markdown preprocessing
  stripFrontmatter?: boolean;   // default: true
}
```

Fields marked as "default: X" mean the framework substitutes X when the
field is absent OR null OR falsy. Pass an explicit value to override.

Path resolution: all relative paths resolve against the directory
containing the config file (NOT process.cwd). Absolute paths work too.

## 6. Integration playbook (new-repo onboarding)

Prescriptive checklist for an AI agent dropping tutorial-forge into a repo
that doesn't have it:

### Step 1 — Copy the framework

```bash
cp -r <source-repo>/scripts/tutorial-forge <target-repo>/scripts/tutorial-forge
```

Alternative: `git subtree add --prefix=scripts/tutorial-forge <source-url> main`.

### Step 2 — Install deps

```bash
cd <target-repo>/scripts/tutorial-forge
npm install
```

Note: this downloads Chromium (~150 MB). Agents working in sandboxed
environments MUST confirm the install succeeds BEFORE Step 4. On failure:
check `~/.cache/puppeteer/` for a chrome directory; if missing, re-run
`npx puppeteer browsers install chrome` from the forge directory.

### Step 3 — Add `.gitignore` entry

Append to repo `.gitignore`:
```
scripts/tutorial-forge/node_modules/
```

### Step 4 — Create `tutorial.config.json`

Minimum viable config:

```json
{
  "deck": "docs/tutorial.md",
  "output": "build/tutorial-static",
  "title": "My Tutorial"
}
```

Create at the repo root (same dir as `.gitignore`, not inside the forge).

### Step 5 — Author the deck

Create `docs/tutorial.md` with standard markdown. Framework conventions:

- Fenced code blocks with language tag get code-theme highlighting.
- ```` ```mermaid ```` fences get pre-rendered to SVG + interactive viewer.
- `<!-- .slide: id="pinned-anchor" -->` on its own line BEFORE a heading
  pins the heading's HTML id (useful for TOC links).
- `note:` on its own line after main content starts a speaker-note block
  which becomes a collapsible `<details>` at the end of that section.
- `---` on its own line separates sections visually.

See §7 for exact author-side rules.

### Step 6 — Verify the build

```bash
node scripts/tutorial-forge/cli.mjs build --config tutorial.config.json
```

Expected success output: lines prefixed `[tutorial-forge]`, final line
`DONE: N files, X.XX MB total`.

Serve + browser-check:

```bash
cd build/tutorial-static && python3 -m http.server 8000
# open http://localhost:8000/
```

### Step 7 — Add Makefile targets (optional but recommended)

```makefile
.PHONY: sync-tutorial-embed verify-tutorial-embed
sync-tutorial-embed:
	@test -d scripts/tutorial-forge/node_modules \
		|| (echo "run: cd scripts/tutorial-forge && npm ci" && exit 1)
	@node scripts/tutorial-forge/cli.mjs build --config tutorial.config.json

verify-tutorial-embed:
	@python3 scripts/verify_tutorial_embed.py
```

Copy `scripts/verify_tutorial_embed.py` from the mdemg repo as a reference
drift-gate implementation. Adjust constants if your layout differs; the
script reads `tutorial.config.json` so it auto-adapts to CSS/viewer
override paths.

### Step 8 — CI wire

In the repo's CI workflow, add a step that runs `make verify-tutorial-embed`.
This fails fast if someone edits the deck without re-running the sync.

## 7. Deck authoring reference

### 7.1 Headings

Any `#`-`######` heading gets an `id` attribute. Resolution precedence:

1. **Explicit pin**: a `<!-- .slide: id="X" -->` directive IMMEDIATELY
   preceding (within the same section) will attach `id="X"` to the heading
   whose plain text matches, in source order.
2. **Auto-slug**: lowercased, stripped of non-word chars, whitespace →
   dashes, deduplicated with `-2`, `-3`, … suffixes.

**Agent must know**: TOC fragment links like `#my-section` work ONLY if the
heading they target has that exact id. Changing a heading's wording
without a pin breaks any TOC entry pointing at the auto-slug.

### 7.2 Speaker notes

A line consisting of exactly `note:` (optional trailing whitespace, must be
at start of line) after section content starts a note block. Everything
after it (up to the next `\n---\n` boundary) becomes `<details
class="speaker-notes"><summary>Speaker notes</summary>…`.

### 7.3 Mermaid

Standard fenced blocks only; `mermaid` is the required language tag.
Framework pre-renders ALL diagrams at build time with these effective
settings (not configurable at the deck level — change in `renderers/
mermaid.mjs`):

- `theme`: light (`default`), overridable via `config.mermaid.theme`.
- `flowchart.htmlLabels: true` — labels become foreignObject HTML. This
  is permissive on special chars but means the rendered SVG embeds HTML.
- `flowchart.curve: 'basis'` — smooth curved edges.
- `useMaxWidth: false` — SVG keeps its natural computed width; the
  `.svg-figure` wrapper + `object-fit: contain` scales it to the viewer frame.

**Author rules that improve rendered quality**:

| Pattern | Behavior | Fix if awkward |
| --- | --- | --- |
| `flowchart LR` with many nodes | Wide-aspect SVG (ratio > 3×), text crushes | Switch to `TD` |
| `[Label with (parens)]` | Framework auto-quotes to `["..."]`; harmless | Quote explicitly: `["Label with (parens)"]` |
| `[Line one<br/>Line two]` | `<br/>` auto-converted to `\n` | OK, no change needed |
| Nodes using `&` as "and" | Auto-quoted; safe | OK |
| Ultra-long labels | SVG natural width inflates; text may crush in viewer frame | Shorten labels or split node |

### 7.4 Images

Standard markdown image syntax works. Images are NOT auto-wrapped in the
viewer; only `` ```mermaid `` blocks and files emitted into `assets/svg/`
get the `.svg-figure` treatment. For a hand-authored diagram that should
get the viewer treatment, write it as raw HTML in the deck:

```markdown
<figure class="svg-figure" tabindex="0" aria-label="My diagram">
  <img src="assets/svg/my-diagram.svg" alt="My diagram" />
</figure>
```

Place `my-diagram.svg` under `config.assetsDir/svg/` so it lands in the
output.

### 7.5 Slide-breaks

A line consisting of exactly `---` splits logical sections. Rendered as
`<hr class="slide-break" />` — a subtle dashed horizontal rule.

### 7.6 Acronym expansion

Author normal prose. The framework finds and expands acronyms matching the
merged dictionary (default + `config.acronyms.extra`, minus
`config.acronyms.disabled`). Pre-existing `"Expansion (ACRONYM)"` forms
authored by hand count toward the per-acronym budget — the transform
leaves them alone.

To prevent expansion of a specific project-internal name (e.g., `MDEMG`,
`JIMINY`, `RSIC`), add it to `config.acronyms.disabled`. It will never be
expanded regardless of the default dictionary.

## 8. Verification contracts

An agent that successfully built a tutorial via tutorial-forge can
verify the result shape with these invariants:

### 8.1 File-count invariants

For a deck with M `` ```mermaid `` blocks and no hand-authored SVGs:

- `index.html` exists.
- `SOURCE_SHA256.txt` exists with keys `deck_sha256`, `css_sha256`,
  `svg_viewer_sha256`, each 64-hex-char.
- `assets/css/slides-overrides.css`, `assets/css/zenburn.min.css` exist.
- `assets/js/svg-viewer.js` exists.
- `assets/svg/mermaid-01.svg` … `assets/svg/mermaid-${M.pad(2)}.svg` exist.

### 8.2 HTML shape invariants

```javascript
const h = readIndexHtml();
assert h.includes('<main class="deck">');
assert !h.includes('mermaid.min.js');           // runtime NOT shipped
assert !h.includes('<div class="reveal">');     // reveal.js NOT present
const svgFigs = h.match(/class="svg-figure"/g).length;
const mermaidRefs = h.match(/mermaid-\d+\.svg/g).length;
assert svgFigs === mermaidRefs + hand_authored_count;
assert headingIds >= 1;  // every heading gets an id
```

### 8.3 Idempotency invariant

Running `build` twice in a row produces byte-identical `index.html`:

```bash
make sync-tutorial-embed && sha256sum output/index.html
make sync-tutorial-embed && sha256sum output/index.html
# the two sums MUST match
```

Mermaid SVGs may differ by a few bytes between runs due to coordinate
precision noise in Chromium's layout engine; this is tolerated and NOT a
regression.

### 8.4 Acronym-expansion invariant

For every acronym in the merged dict, the string `"${expansion} (${ACRONYM})"`
appears **≤ expandCount times** in the output HTML. If an acronym does not
appear at all in the deck, it appears 0 times.

## 9. Common failure modes

| Signature | Root cause | Fix |
| --- | --- | --- |
| `mermaid render failed for block N: Parse error on line X: ... got 'PS'` | Unquoted `(` inside `[label]`; parser reads as stadium shape | Framework should auto-quote; if error persists, quote the label manually: `A["label with (parens)"]` |
| `mermaid render failed for block N: Expecting 'SEMI', got 'STR'` | Label contains unescaped `"` | Escape with `&quot;` or use single quotes outside: `[Label with \"quotes\"]` → `['Label with "quotes"']` |
| `FATAL: deck source not found: <path>` | `config.deck` path wrong or deck missing | Check `tutorial.config.json`; path is relative to config file dir |
| `FATAL: SVG viewer not found` | `config.svgViewerPath` points nowhere; framework default also missing | Either fix the config path or re-install forge deps |
| `tutorial-forge deps not installed` (from Makefile) | `scripts/tutorial-forge/node_modules/` missing | `cd scripts/tutorial-forge && npm ci` |
| Build hangs / Chromium never launches | Puppeteer cache missing; `~/.cache/puppeteer/chrome*` absent | `npx puppeteer browsers install chrome` from forge dir |
| `make verify-tutorial-embed` reports DRIFT after a deck edit | Normal — the drift gate is doing its job | Run `make sync-tutorial-embed` + stage the regenerated `staged/` dir + commit |
| CI 404 on `/ui/tutorial/assets/js/svg-viewer.js` after deploy | Build never ran OR `go:embed` didn't pick up the staged dir | Verify `internal/cli/tutorial_templates/staged/` contains the files locally; rebuild binary |
| Mermaid diagram renders but text is tiny | Wide-aspect SVG (viewBox width ≫ height) scaled into `.svg-figure` frame | Convert `flowchart LR` → `TD` in deck source |
| Mermaid diagram text wrong color (white on white, black on black) | Mismatched `config.mermaid.theme` vs `.svg-figure` background | Default theme is light (black text on white); if you set `theme: 'dark'`, override CSS so `.svg-figure` has a dark background |
| Acronym expanded wrong (e.g., `CMS` means "Cognitive Memory Substrate" not "Content Management System") | Framework default collides with project-internal meaning | Add to `config.acronyms.extra` to override, OR add to `config.acronyms.disabled` and let surrounding prose define it |

## 10. Decision table: when the author does X, framework does Y

| Deck pattern | Framework behavior |
| --- | --- |
| `# Heading` (no pin above) | Auto-slug id = `heading`; dedup suffix if collides |
| `<!-- .slide: id="foo" -->\n# Heading` | id = `foo` (explicit pin) |
| ```` ```mermaid\nfoo\n``` ```` | Rendered to `assets/svg/mermaid-01.svg`; replaced with `.svg-figure` wrapper |
| `` `inline code` `` | Passthrough; acronym transform skips contents |
| `[link](https://…/UVTS)` | `UVTS` in URL is skipped (link-URL mask); link text IS scanned |
| `LLM` first time (new deck) | Expanded to `Large Language Model (LLM)` |
| `LLM` third time | Left as `LLM` (budget exhausted) |
| Author-written `Large Language Model (LLM)` | Counts toward LLM's budget of 2 (idempotency) |
| ```` ```python\nLLM = ... \n``` ```` | `LLM` not expanded (fenced-code mask) |
| `<!-- a comment -->` on its own line | Line passthrough; acronym transform skips contents |
| `note:` block | Wrapped in `<details class="speaker-notes">` |
| `---` on its own line | Rendered as `<hr class="slide-break" />` |
| YAML frontmatter at top | Stripped by default (`config.stripFrontmatter: true`) |

## 11. CI integration pattern

### 11.1 Drift gate

Purpose: fail CI if someone edits the deck without re-running the sync.
Reference impl: `scripts/verify_tutorial_embed.py` in the mdemg repo. It
reads `tutorial.config.json`, computes current SHAs of
deck/CSS/viewer, compares to `SOURCE_SHA256.txt` in the output dir.

Exit codes:
- `0` — no drift.
- `1` — drift (or breadcrumb missing).

Wire example (GitHub Actions):

```yaml
- name: Install tutorial-forge
  run: cd scripts/tutorial-forge && npm ci
- name: Build tutorial
  run: make sync-tutorial-embed
- name: Verify tutorial embed
  run: make verify-tutorial-embed
```

### 11.2 Diff-aware drift gate

The breadcrumb pattern makes the gate trivially diffable. Add a `git
diff --exit-code SOURCE_SHA256.txt` step to catch drift that was committed
to the output dir with stale SHAs:

```yaml
- name: Confirm staged embed is current
  run: |
    make sync-tutorial-embed
    git diff --exit-code internal/cli/tutorial_templates/staged/
```

## 12. Rollback

| Scope | Procedure |
| --- | --- |
| One bad build | `rm -rf <config.output> && make sync-tutorial-embed` |
| One bad commit | `git revert <sha>` + `make sync-tutorial-embed` + commit |
| Framework upgrade broke a build | `cd scripts/tutorial-forge && git checkout <known-good-sha> -- .` + `npm ci` + rebuild |
| Deck doesn't need the framework anymore | Delete `scripts/tutorial-forge/`, `tutorial.config.json`, output dir, Makefile targets, drift verifier. Deck source + any hand-authored assets stay fine standalone. |

## 13. Mdemg-specific notes (REFERENCE IMPLEMENTATION)

Mdemg (`https://github.com/reh3376/mdemg`) is the primary consumer. Study it
for the full integration pattern:

- Config: `tutorial.config.json` at repo root.
- Deck: `docs/onboarding/mdemg-onboarding-deck.md`.
- CSS override: `docs/onboarding/slides-overrides.css`.
- Viewer override: `docs/onboarding/svg-viewer.js`.
- Hand-authored assets: `docs/onboarding/assets/svg/u4-how-jiminy-thinks.svg`.
- Output: `internal/cli/tutorial_templates/staged/`.
- `//go:embed` directive: `internal/cli/tutorial_templates/embed.go`.
- HTTP handler: `internal/api/handlers_tutorial.go` (serves at `/ui/tutorial/`).
- Embed invariant tests: `internal/cli/tutorial_templates/embed_test.go`.
- Handler tests: `internal/api/handlers_tutorial_test.go`.
- Drift verifier: `scripts/verify_tutorial_embed.py`.
- Makefile targets: `sync-tutorial-embed`, `verify-tutorial-embed`.

Mdemg's acronym config disables expansion for 5 project-internal names
(`MDEMG`, `JIMINY`, `RSIC`, `J17`, `MLX`) and adds 7 project-specific
expansions (`CMS`, `UATS`, `UVTS`, `UBENCH`, `ULTS`, `UAITS`, `UXTS`). This
is the canonical pattern for a repo with its own naming conventions.

## 14. Framework versioning

The framework ships at `scripts/tutorial-forge/` within a consumer repo —
no npm registry publish yet. Agents upgrading should:

1. Pull the latest `scripts/tutorial-forge/` from the source repo.
2. Diff `package.json` for dep version changes.
3. `cd scripts/tutorial-forge && npm install` to update node_modules.
4. Run `make sync-tutorial-embed` to regenerate the output.
5. Run `make verify-tutorial-embed` to confirm.
6. Commit the regenerated output.

Breaking changes are indicated by MINOR bumps in `package.json`'s `version`
field (currently `0.1.0`). The framework is pre-1.0; any change may break.

## 15. What this document is NOT

- Not a tutorial on how to USE the shipped MDEMG tutorial. For that, run
  mdemg and visit `/ui/tutorial/`.
- Not a design critique or rationale document. For the "why we pivoted from
  reveal.js" story, see commit message `0f12fbfa` in the mdemg repo.
- Not a changelog. See git log.
- Not a replacement for `README.md`. The README is human-oriented; this FSD
  is agent-oriented. They cover the same system from different angles —
  neither one is the subset of the other.

---

**End of FSD.** Agents working on tutorial-forge should keep this document
synchronized with code changes. If a code change contradicts a §4 "load-bearing"
contract, update both in the same commit.
