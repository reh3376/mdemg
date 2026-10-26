#!/usr/bin/env node
// ONBOARDING-TUTORIAL-SERVE-001 E1: build-time export of the onboarding deck
// to a static reveal.js HTML bundle under internal/cli/tutorial_templates/staged/.
//
// Invariants:
// - Source-of-truth is docs/onboarding/mdemg-onboarding-deck.md at the repo root.
// - Output lands at internal/cli/tutorial_templates/staged/.
// - mermaid.min.js is NOT bundled by reveal-md --static; we copy it explicitly
//   from scripts/tutorial/node_modules/mermaid/dist/ so the exported HTML renders
//   diagrams offline (R2 risk in sprint_plan.md).
// - The slides-overrides.css fix for bottom clipping travels via --css.
// - CI invokes this via `make sync-tutorial-embed`. The CI drift gate
//   (`make verify-tutorial-embed`) fails merge if the deck source changed
//   without re-running the sync. Mirrors the GRAFANA-SHIP pattern
//   (RELEASE-HYGIENE-001).
//
// Not meant to be invoked directly by operators. Dev-tool only.

import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, rmSync, copyFileSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { join, dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(SCRIPT_DIR, '..', '..');
const DECK_SRC = join(REPO_ROOT, 'docs', 'onboarding', 'mdemg-onboarding-deck.md');
const CSS_SRC = join(REPO_ROOT, 'docs', 'onboarding', 'slides-overrides.css');
const SVG_VIEWER_SRC = join(REPO_ROOT, 'docs', 'onboarding', 'svg-viewer.js');
const STAGED_DIR = join(REPO_ROOT, 'internal', 'cli', 'tutorial_templates', 'staged');
const MERMAID_SRC = join(SCRIPT_DIR, 'node_modules', 'mermaid', 'dist', 'mermaid.min.js');
const REVEAL_MD_BIN = join(SCRIPT_DIR, 'node_modules', '.bin', 'reveal-md');

function fail(msg) {
  console.error(`[export_tutorial] FATAL: ${msg}`);
  process.exit(1);
}

function info(msg) {
  console.log(`[export_tutorial] ${msg}`);
}

if (!existsSync(DECK_SRC)) fail(`deck source not found: ${DECK_SRC}`);
if (!existsSync(CSS_SRC)) fail(`CSS override not found: ${CSS_SRC}`);
if (!existsSync(SVG_VIEWER_SRC)) fail(`svg-viewer.js not found: ${SVG_VIEWER_SRC}`);
if (!existsSync(REVEAL_MD_BIN)) fail(`reveal-md not installed. Run: cd scripts/tutorial && npm ci`);
if (!existsSync(MERMAID_SRC)) fail(`mermaid.min.js not installed. Run: cd scripts/tutorial && npm ci`);

// Clean + recreate the staged dir. Pre-existing content is always discarded —
// embed.go reads the whole tree, so stale files would land in the binary.
if (existsSync(STAGED_DIR)) {
  rmSync(STAGED_DIR, { recursive: true, force: true });
}
mkdirSync(STAGED_DIR, { recursive: true });

info(`exporting ${DECK_SRC}`);
info(`  --> ${STAGED_DIR}`);

// reveal-md bakes argv paths into index.html verbatim (as both the <link>
// stylesheet href AND an embedded JSON blob of CLI args). An absolute path
// here would make the embed byte-differ across machines (local vs CI vs other
// operators), breaking the CI drift check. Pass RELATIVE paths from the repo
// root; cwd is REPO_ROOT so reveal-md resolves them correctly.
const DECK_REL = 'docs/onboarding/mdemg-onboarding-deck.md';
const CSS_REL = 'docs/onboarding/slides-overrides.css';
const STAGED_REL = 'internal/cli/tutorial_templates/staged';
try {
  execFileSync(REVEAL_MD_BIN, [
    DECK_REL,
    '--static', STAGED_REL,
    '--css', CSS_REL,
    '--theme', 'night',
    '--title', 'MDEMG Tutorial',
  ], { stdio: 'inherit', cwd: REPO_ROOT });
} catch (e) {
  fail(`reveal-md static export failed: ${e.message}`);
}

// reveal-md references ./mermaid/dist/mermaid.min.js from the generated HTML
// but does NOT bundle the file into --static output. Copy it ourselves so
// the exported bundle renders diagrams offline (operator LANs + CI + air-gapped).
const mermaidDestDir = join(STAGED_DIR, 'mermaid', 'dist');
mkdirSync(mermaidDestDir, { recursive: true });
copyFileSync(MERMAID_SRC, join(mermaidDestDir, 'mermaid.min.js'));
info(`bundled mermaid.min.js (${statSync(MERMAID_SRC).size} bytes)`);

// ONBOARDING-TUTORIAL-ENDUSER-001 E5 fix: reveal-md's default mermaid init
// only renders the ".present" slide's diagrams. In reveal.js 5.x `view: scroll`
// mode no slide is marked `.present`, so diagrams never render — operator
// sees raw mermaid source instead of SVG. Patch the generated index.html's
// mermaid init to render ALL diagrams on `ready`.
const indexPath = join(STAGED_DIR, 'index.html');
let indexHtml = readFileSync(indexPath, 'utf8');
const presentMermaidRe = /mermaid\.init\(mermaidOptions, '\.slides > \.present:not\(\.stack\) pre code\.mermaid'\);/;
if (presentMermaidRe.test(indexHtml)) {
  indexHtml = indexHtml.replace(
    presentMermaidRe,
    "mermaid.init(mermaidOptions, '.slides pre code.mermaid');  // ENDUSER-001 E5: render all slides (scroll view compat)"
  );
  info('patched mermaid init for scroll-view compatibility');
} else {
  info('mermaid init pattern not found — reveal-md may have updated; skipping patch');
}

// ONBOARDING-TUTORIAL-ENDUSER-001 SVG viewer: copy the viewer JS into the
// staged bundle and inject a <script> tag so every <figure class="svg-figure">
// becomes an interactive zoom/pan container at page load.
copyFileSync(SVG_VIEWER_SRC, join(STAGED_DIR, 'svg-viewer.js'));
info(`bundled svg-viewer.js (${statSync(SVG_VIEWER_SRC).size} bytes)`);
const viewerScriptTag = '  <script defer src="svg-viewer.js"></script>\n</body>';
if (indexHtml.includes('</body>') && !indexHtml.includes('svg-viewer.js')) {
  indexHtml = indexHtml.replace('</body>', viewerScriptTag);
  info('injected svg-viewer.js <script> tag before </body>');
} else if (indexHtml.includes('svg-viewer.js')) {
  info('svg-viewer.js already referenced in index.html; skipping injection');
} else {
  fail('index.html has no </body> tag — cannot inject svg-viewer.js reference');
}

writeFileSync(indexPath, indexHtml);

// Write a SOURCE_SHA256 breadcrumb so make verify-tutorial-embed can detect
// drift (deck source changed without re-running the sync). The verifier
// recomputes the SHA of DECK_SRC + CSS_SRC and compares.
const deckSha = createHash('sha256').update(readFileSync(DECK_SRC)).digest('hex');
const cssSha = createHash('sha256').update(readFileSync(CSS_SRC)).digest('hex');
const viewerSha = createHash('sha256').update(readFileSync(SVG_VIEWER_SRC)).digest('hex');
writeFileSync(
  join(STAGED_DIR, 'SOURCE_SHA256.txt'),
  `# ONBOARDING-TUTORIAL-SERVE-001 drift breadcrumb\n` +
  `# verified by scripts/verify_tutorial_embed.py (CI gate)\n` +
  `deck_sha256=${deckSha}\n` +
  `css_sha256=${cssSha}\n` +
  `svg_viewer_sha256=${viewerSha}\n` +
  `deck_path=docs/onboarding/mdemg-onboarding-deck.md\n` +
  `css_path=docs/onboarding/slides-overrides.css\n` +
  `svg_viewer_path=docs/onboarding/svg-viewer.js\n`
);
info(`wrote SOURCE_SHA256.txt (deck=${deckSha.slice(0,12)}... css=${cssSha.slice(0,12)}... viewer=${viewerSha.slice(0,12)}...)`);

// Count what landed for the sprint-post metrics.
function countFiles(dir) {
  let n = 0; let bytes = 0;
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, entry.name);
    if (entry.isDirectory()) {
      const [cn, cb] = countFiles(p);
      n += cn; bytes += cb;
    } else {
      n += 1; bytes += statSync(p).size;
    }
  }
  return [n, bytes];
}
const [fileCount, totalBytes] = countFiles(STAGED_DIR);
info(`DONE: ${fileCount} files, ${(totalBytes / (1024*1024)).toFixed(2)} MB total`);
