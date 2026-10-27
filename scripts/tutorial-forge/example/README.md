# tutorial-forge example

Minimal working example showing the framework's defaults with ZERO
overrides. A fresh repo can copy this directory as its starting point.

## Files

- `deck.md` — tiny demo deck with 5 sections and 2 Mermaid diagrams.
- `tutorial.config.json` — minimum valid config: `deck`, `output`, `title`.

## Build

From THIS directory:

```bash
node ../cli.mjs build --config tutorial.config.json
```

That writes a self-contained static site to `./dist/`. Serve it with
anything static:

```bash
cd dist && python3 -m http.server 8000
# open http://localhost:8000/
```

## What you should see

Open the result in a browser at viewport ≥ 980px wide:

- **Left-margin sidebar** headed "Contents" with 5 Sections entries
  (Introduction, Memory Model, Hebbian Learning, Try It Yourself, Where
  To Go From Here).
- A **Graphs** sub-section listing the 2 pre-rendered Mermaid diagrams.
- As you scroll, the current section's TOC entry highlights (border-left
  accent + colored text).
- Clicking any TOC entry jumps to that section.
- At viewport < 980px the TOC hides automatically.

## What you edited

Just two files. The framework defaults supplied:

- The CSS (`scripts/tutorial-forge/templates/default.css`).
- The interactive SVG viewer (`scripts/tutorial-forge/templates/svg-viewer.js`).
- The acronym dictionary (`scripts/tutorial-forge/defaults/acronyms.json`).
- The highlight.js code theme and the favicon.

To customize any of them, add `cssPath` / `svgViewerPath` / `iconPath` /
`acronyms.extra` fields to `tutorial.config.json`. See the schema in
`../README.md` or the agent reference in `../FSD.md`.
