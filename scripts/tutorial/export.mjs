#!/usr/bin/env node
// ONBOARDING-TUTORIAL-ENDUSER-001 (2026-10-26 pivot): build-time export of
// the onboarding deck as a REAL HTML/CSS long-form document — not a reveal.js
// slide deck.
//
// Why this changed: the original pipeline (reveal-md + reveal.js scroll view)
// treated 235 sections of continuous engineering content as slides stitched
// together, and every layout issue (heading clip, overflow bleed, snap-to-slide,
// bounded figure sizing) traced back to that same architectural mismatch.
// Operator directive 2026-10-26: stop fighting the slideshow model, build a
// real HTML/CSS page.
//
// Output is a tutorial PACKAGE — one HTML file + one aux directory:
//
//   staged/
//     index.html                 the deck as one long readable document
//     favicon.ico
//     SOURCE_SHA256.txt          drift breadcrumb for CI gate
//     assets/
//       css/
//         slides-overrides.css   the deck's CSS override
//         zenburn.min.css        highlight.js code theme
//       js/
//         mermaid.min.js         diagram renderer
//         svg-viewer.js          interactive zoom/pan container
//       svg/                     hand-authored SVG diagrams
//
// The HTML references assets via relative paths (`assets/js/...`), so the
// bundle works at any serving prefix and the `//go:embed all:staged` snapshot
// is self-contained.

import { createHash } from 'node:crypto';
import {
  cpSync,
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { marked } from 'marked';

const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(SCRIPT_DIR, '..', '..');
const DECK_SRC = join(REPO_ROOT, 'docs', 'onboarding', 'mdemg-onboarding-deck.md');
const CSS_SRC = join(REPO_ROOT, 'docs', 'onboarding', 'slides-overrides.css');
const SVG_VIEWER_SRC = join(REPO_ROOT, 'docs', 'onboarding', 'svg-viewer.js');
const SVG_DIR_SRC = join(REPO_ROOT, 'docs', 'onboarding', 'assets', 'svg');
const STAGED_DIR = join(REPO_ROOT, 'internal', 'cli', 'tutorial_templates', 'staged');
const MERMAID_SRC = join(SCRIPT_DIR, 'node_modules', 'mermaid', 'dist', 'mermaid.min.js');
const HLJS_STYLE_SRC = join(
  SCRIPT_DIR,
  'node_modules',
  'highlight.js',
  'styles',
  'base16',
  'zenburn.min.css',
);
const FAVICON_SRC = join(SCRIPT_DIR, 'node_modules', 'reveal-md', 'lib', 'favicon.ico');

function fail(msg) {
  console.error(`[export_tutorial] FATAL: ${msg}`);
  process.exit(1);
}
function info(msg) {
  console.log(`[export_tutorial] ${msg}`);
}

for (const [label, path] of [
  ['deck source', DECK_SRC],
  ['CSS override', CSS_SRC],
  ['svg-viewer', SVG_VIEWER_SRC],
  ['svg dir', SVG_DIR_SRC],
  ['mermaid', MERMAID_SRC],
  ['highlight.js style', HLJS_STYLE_SRC],
]) {
  if (!existsSync(path)) fail(`${label} not found: ${path}`);
}

// ---------- Clean + recreate staged dir ----------
if (existsSync(STAGED_DIR)) rmSync(STAGED_DIR, { recursive: true, force: true });
mkdirSync(STAGED_DIR, { recursive: true });
info(`writing to ${STAGED_DIR}`);

// ---------- Read + preprocess markdown ----------
let md = readFileSync(DECK_SRC, 'utf-8');

// Strip YAML frontmatter.
md = md.replace(/^---\n[\s\S]*?\n---\n+/, '');

// Rewrite in-deck relative paths from the authoring layout
// (`./assets/svg/foo.svg`) to the package layout (`assets/svg/foo.svg`).
md = md.replace(/\.\/assets\/svg\//g, 'assets/svg/');

// Preprocess slide-specific syntax, chunk-by-chunk. `---` on its own line
// separates slides in the source. We treat each chunk as one logical section;
// the directive `<!-- .slide: id="X" -->` at the top of a chunk pins that id
// onto the chunk's first heading so TOC fragment links keep working.
const slideChunks = md.split(/\n---\n/);
info(`parsed ${slideChunks.length} slide chunks from deck`);

// Collect explicit `<!-- .slide: id -->` directives and the heading text they
// each pin to, so after marked renders the whole document we can inject the
// explicit id onto the correct <hN>. (We cannot replace the heading with raw
// HTML during preprocessing — marked would treat the inlined <hN> as an HTML
// block and refuse to parse the following markdown paragraph.)
const explicitPins = []; // [{ id, headingText }]

const processedChunks = slideChunks.map((chunk) => {
  const idMatch = chunk.match(/<!--\s*\.slide:\s*id="([^"]+)"\s*-->/);
  const slideId = idMatch ? idMatch[1] : null;

  // Remove the slide directive and any sibling HTML comments.
  let body = chunk.replace(/<!--\s*\.slide:[\s\S]*?-->\s*/g, '');

  // Record the first heading's plain-text so we can re-attach the explicit id
  // after marked renders the body.
  if (slideId) {
    const headMatch = body.match(/^(#{1,6})\s+(.+?)\s*$/m);
    if (headMatch) explicitPins.push({ id: slideId, headingText: headMatch[2].trim() });
  }

  // Pull speaker-note blocks (lines after `note:`) into a <details>.
  const noteMatch = body.match(/\n*note:\s*\n([\s\S]*?)$/);
  let noteHtml = '';
  if (noteMatch) {
    body = body.slice(0, noteMatch.index).trimEnd();
    const noteBodyHtml = marked.parse(noteMatch[1].trim(), { gfm: true, breaks: false });
    noteHtml = `\n<details class="speaker-notes"><summary>Speaker notes</summary>\n${noteBodyHtml}</details>\n`;
  }

  // Turn ```mermaid fences into native `<div class="mermaid">` so mermaid.js
  // auto-renders them. Marked would otherwise wrap in <pre><code>.
  body = body.replace(
    /```mermaid\s*\n([\s\S]*?)\n```/g,
    (_m, code) => `<div class="mermaid">\n${code}\n</div>`,
  );

  return body + noteHtml;
});

// Rejoin with a subtle visual separator between logical sections.
const combinedMd = processedChunks.join('\n\n<hr class="slide-break" />\n\n');

// ---------- Convert markdown → HTML ----------
marked.setOptions({ gfm: true, breaks: false });
let bodyHtml = marked.parse(combinedMd);
info(`markdown → HTML conversion complete (${bodyHtml.length} bytes)`);

// ---------- Post-process: auto-slug every <hN> + apply explicit pins ----------
// Any heading without an id gets a slug derived from its text content. Explicit
// pins from `<!-- .slide: id -->` directives take precedence over auto-slugs.
// Deck-wide uniqueness: if two headings slug the same, suffix `-2`, `-3`, ...
const seenSlugs = new Map();
function slugify(text) {
  const base = String(text)
    .toLowerCase()
    .trim()
    .replace(/[^\w\s-]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '');
  if (!base) return '';
  const count = (seenSlugs.get(base) ?? 0) + 1;
  seenSlugs.set(base, count);
  return count === 1 ? base : `${base}-${count}`;
}

// Prime the slug table with the explicit-pin ids so auto-slug can't collide.
for (const pin of explicitPins) seenSlugs.set(pin.id, 1);

// Strip HTML tags from a string. Loops until stable because a single-pass
// regex can be defeated by overlapping tag sequences like `<<script>script>`
// (CodeQL js/incomplete-multi-character-sanitization). The output of this
// helper only feeds the slugify() pipeline which restricts to [a-z0-9-], so
// there's no XSS surface — but the idempotent loop matches CodeQL's expected
// sanitization pattern.
function stripTags(s) {
  let prev;
  let out = String(s);
  do {
    prev = out;
    out = out.replace(/<[^>]*>/g, '');
  } while (out !== prev);
  return out;
}

// First pass: attach explicit pin ids to matching headings. We walk the pin
// queue and attach each in-order to the FIRST unlocked heading that matches
// its plain text (preserving source order).
let pinCursor = 0;
bodyHtml = bodyHtml.replace(
  /<(h[1-6])(?![^>]*\sid=)>([\s\S]*?)<\/\1>/g,
  (match, tag, inner) => {
    const plain = stripTags(inner).trim();
    while (pinCursor < explicitPins.length) {
      const pin = explicitPins[pinCursor];
      if (pin.headingText === plain) {
        pinCursor += 1;
        return `<${tag} id="${pin.id}">${inner}</${tag}>`;
      }
      // No match here; the pin's heading hasn't appeared yet, so leave the
      // cursor where it is and let later matches drain pending pins.
      break;
    }
    return match;
  },
);

// Second pass: auto-slug any heading that still lacks an id.
bodyHtml = bodyHtml.replace(
  /<(h[1-6])(?![^>]*\sid=)>([\s\S]*?)<\/\1>/g,
  (match, tag, inner) => {
    const plain = stripTags(inner).trim();
    const id = slugify(plain);
    return id ? `<${tag} id="${id}">${inner}</${tag}>` : match;
  },
);

// ---------- Assemble HTML shell ----------
const indexHtml = `<!doctype html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>MDEMG Tutorial</title>
  <link rel="icon" href="favicon.ico" />
  <link rel="stylesheet" href="assets/css/zenburn.min.css" />
  <link rel="stylesheet" href="assets/css/slides-overrides.css" />
</head>
<body>
  <main class="deck">
${bodyHtml}
  </main>
  <script src="assets/js/mermaid.min.js"></script>
  <script>
    mermaid.initialize({
      startOnLoad: true,
      theme: 'dark',
      securityLevel: 'loose',
      flowchart: { useMaxWidth: true, htmlLabels: true },
    });
  </script>
  <script defer src="assets/js/svg-viewer.js"></script>
</body>
</html>
`;

writeFileSync(join(STAGED_DIR, 'index.html'), indexHtml);
info(`wrote index.html (${indexHtml.length} bytes)`);

// ---------- Copy aux assets into the tutorial package ----------
mkdirSync(join(STAGED_DIR, 'assets', 'css'), { recursive: true });
mkdirSync(join(STAGED_DIR, 'assets', 'js'), { recursive: true });

cpSync(CSS_SRC, join(STAGED_DIR, 'assets', 'css', 'slides-overrides.css'));
info('bundled assets/css/slides-overrides.css');

cpSync(HLJS_STYLE_SRC, join(STAGED_DIR, 'assets', 'css', 'zenburn.min.css'));
info('bundled assets/css/zenburn.min.css');

cpSync(MERMAID_SRC, join(STAGED_DIR, 'assets', 'js', 'mermaid.min.js'));
info(`bundled assets/js/mermaid.min.js (${statSync(MERMAID_SRC).size} bytes)`);

cpSync(SVG_VIEWER_SRC, join(STAGED_DIR, 'assets', 'js', 'svg-viewer.js'));
info(`bundled assets/js/svg-viewer.js (${statSync(SVG_VIEWER_SRC).size} bytes)`);

cpSync(SVG_DIR_SRC, join(STAGED_DIR, 'assets', 'svg'), { recursive: true });
info('bundled assets/svg/ (hand-authored diagrams)');

if (existsSync(FAVICON_SRC)) {
  cpSync(FAVICON_SRC, join(STAGED_DIR, 'favicon.ico'));
}

// ---------- Drift breadcrumb ----------
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
    `svg_viewer_path=docs/onboarding/svg-viewer.js\n`,
);
info(
  `wrote SOURCE_SHA256.txt (deck=${deckSha.slice(0, 12)}... css=${cssSha.slice(0, 12)}... viewer=${viewerSha.slice(0, 12)}...)`,
);

// ---------- Tally ----------
function countFiles(dir) {
  let n = 0;
  let bytes = 0;
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, entry.name);
    if (entry.isDirectory()) {
      const [cn, cb] = countFiles(p);
      n += cn;
      bytes += cb;
    } else {
      n += 1;
      bytes += statSync(p).size;
    }
  }
  return [n, bytes];
}
const [fileCount, totalBytes] = countFiles(STAGED_DIR);
info(`DONE: ${fileCount} files, ${(totalBytes / (1024 * 1024)).toFixed(2)} MB total`);
