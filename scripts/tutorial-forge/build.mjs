// tutorial-forge build orchestrator. Config-driven, repo-agnostic — reads a
// tutorial.config.json (or any JSON file whose shape matches TutorialConfig)
// and emits a self-contained static HTML package at `output`.
//
// Package layout emitted:
//   <output>/
//     index.html                    deck as one long readable document
//     favicon.ico                   (if config.iconPath set)
//     SOURCE_SHA256.txt             drift breadcrumb for CI gate
//     assets/
//       css/
//         slides-overrides.css      (config.cssPath or framework default)
//         zenburn.min.css           highlight.js code theme
//       js/
//         svg-viewer.js             (config.svgViewerPath or framework default)
//       svg/
//         mermaid-NN.svg            auto-generated from ```mermaid fences
//         <hand-authored>.svg       (merged from config.assetsDir if present)
//
// References inside index.html are relative so the bundle works at any
// serving prefix (/, /ui/tutorial/, /docs/, etc.).

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

import { renderMermaidSources } from './renderers/mermaid.mjs';
import { expandAcronyms, loadAcronyms } from './transforms/acronyms.mjs';

const PKG_DIR = dirname(fileURLToPath(import.meta.url));
const FRAMEWORK_DEFAULTS = {
  css: join(PKG_DIR, 'templates', 'default.css'),
  svgViewer: join(PKG_DIR, 'templates', 'svg-viewer.js'),
  highlightStyle: join(PKG_DIR, 'node_modules', 'highlight.js', 'styles', 'base16', 'zenburn.min.css'),
  mermaid: join(PKG_DIR, 'node_modules', 'mermaid', 'dist', 'mermaid.min.js'),
  // reveal-md ships a tiny favicon we can reuse as the framework default.
  favicon: join(PKG_DIR, 'node_modules', 'reveal-md', 'lib', 'favicon.ico'),
};

export function logInfo(msg) {
  console.log(`[tutorial-forge] ${msg}`);
}
function fail(msg) {
  console.error(`[tutorial-forge] FATAL: ${msg}`);
  process.exit(1);
}

/**
 * Resolve a config path relative to the config file's own directory. Supports
 * absolute paths too. Returns null if the input is null/undefined/empty.
 */
function resolveRelative(baseDir, p) {
  if (!p) return null;
  return resolve(baseDir, p);
}

/**
 * @typedef {object} TutorialConfig
 * @property {string} deck            path to the deck markdown source (required)
 * @property {string} output          path to the output directory (required)
 * @property {string} [title]         <title> text (default: 'Tutorial')
 * @property {string} [cssPath]       override framework default CSS
 * @property {string} [svgViewerPath] override framework default SVG viewer
 * @property {string} [assetsDir]     directory of extra assets (recursively copied to output/assets)
 * @property {string} [iconPath]      favicon.ico path (default: framework favicon)
 * @property {object} [acronyms]      { extra?: {ACRONYM: expansion}, disabled?: [KEY,…], expandCount?: 2 }
 * @property {object} [mermaid]       { theme?: 'default'|'dark', skipRender?: boolean }
 * @property {boolean} [stripFrontmatter] drop a leading YAML frontmatter block (default true)
 */

/**
 * Run the full build. Returns a summary object { files, bytes, mermaidCount, applied }.
 *
 * @param {TutorialConfig} config
 * @param {string} configDir  base dir for resolving relative paths in config
 */
export async function build(config, configDir) {
  if (!config.deck) fail('config.deck is required');
  if (!config.output) fail('config.output is required');

  const DECK_SRC = resolveRelative(configDir, config.deck);
  const OUTPUT_DIR = resolveRelative(configDir, config.output);
  const CSS_SRC = resolveRelative(configDir, config.cssPath) ?? FRAMEWORK_DEFAULTS.css;
  const SVG_VIEWER_SRC = resolveRelative(configDir, config.svgViewerPath) ?? FRAMEWORK_DEFAULTS.svgViewer;
  const ASSETS_DIR = resolveRelative(configDir, config.assetsDir);
  const ICON_SRC = resolveRelative(configDir, config.iconPath) ?? FRAMEWORK_DEFAULTS.favicon;
  const title = config.title ?? 'Tutorial';
  const mermaidOpts = config.mermaid ?? {};
  const acronymsOpts = config.acronyms ?? {};
  const expandCount = acronymsOpts.expandCount ?? 2;
  const stripFrontmatter = config.stripFrontmatter !== false;

  // Preflight — fail fast with clear names for every missing input.
  for (const [label, path] of [
    ['deck source', DECK_SRC],
    ['CSS', CSS_SRC],
    ['SVG viewer', SVG_VIEWER_SRC],
    ['highlight.js style', FRAMEWORK_DEFAULTS.highlightStyle],
    ['mermaid runtime (for renderer)', FRAMEWORK_DEFAULTS.mermaid],
  ]) {
    if (!existsSync(path)) fail(`${label} not found: ${path}`);
  }

  // Clean + recreate output dir.
  if (existsSync(OUTPUT_DIR)) rmSync(OUTPUT_DIR, { recursive: true, force: true });
  mkdirSync(OUTPUT_DIR, { recursive: true });
  logInfo(`writing to ${OUTPUT_DIR}`);

  let md = readFileSync(DECK_SRC, 'utf-8');

  if (stripFrontmatter) {
    md = md.replace(/^---\n[\s\S]*?\n---\n+/, '');
  }

  // Normalize authoring-time relative paths that assume the deck lives next to
  // an `assets/` sibling. The emitted package co-locates assets/ with
  // index.html so references like `./assets/svg/foo.svg` need to drop the `./`.
  md = md.replace(/\.\/assets\//g, 'assets/');

  // Acronym expansion (first N occurrences per acronym).
  const acronyms = loadAcronyms(acronymsOpts);
  const { text: afterAcronyms, applied: acronymsApplied } = expandAcronyms(md, acronyms, expandCount);
  md = afterAcronyms;
  const appliedCount = Object.values(acronymsApplied).reduce((a, b) => a + b, 0);
  if (appliedCount > 0) {
    logInfo(`expanded ${appliedCount} acronym occurrences across ${Object.keys(acronymsApplied).length} distinct terms`);
  }

  // Chunk-wise preprocessing (slide directives, speaker notes, mermaid fences).
  const slideChunks = md.split(/\n---\n/);
  logInfo(`parsed ${slideChunks.length} source chunks`);

  const explicitPins = [];
  const mermaidSources = [];
  const MERMAID_PLACEHOLDER = (n) => `@@MERMAID_PLACEHOLDER_${n}@@`;

  const processedChunks = slideChunks.map((chunk) => {
    const idMatch = chunk.match(/<!--\s*\.slide:\s*id="([^"]+)"\s*-->/);
    const slideId = idMatch ? idMatch[1] : null;
    let body = chunk.replace(/<!--\s*\.slide:[\s\S]*?-->\s*/g, '');

    if (slideId) {
      const headMatch = body.match(/^(#{1,6})\s+(.+?)\s*$/m);
      if (headMatch) explicitPins.push({ id: slideId, headingText: headMatch[2].trim() });
    }

    // Pull `note: ...` blocks into collapsible <details>.
    const noteMatch = body.match(/\n*note:\s*\n([\s\S]*?)$/);
    let noteHtml = '';
    if (noteMatch) {
      body = body.slice(0, noteMatch.index).trimEnd();
      const noteBodyHtml = marked.parse(noteMatch[1].trim(), { gfm: true, breaks: false });
      noteHtml = `\n<details class="speaker-notes"><summary>Speaker notes</summary>\n${noteBodyHtml}</details>\n`;
    }

    // Collect ```mermaid blocks for build-time rendering.
    body = body.replace(/```mermaid\s*\n([\s\S]*?)\n```/g, (_m, code) => {
      const idx = mermaidSources.length;
      mermaidSources.push(code);
      return MERMAID_PLACEHOLDER(idx);
    });

    return body + noteHtml;
  });

  const combinedMd = processedChunks.join('\n\n<hr class="slide-break" />\n\n');

  marked.setOptions({ gfm: true, breaks: false });
  let bodyHtml = marked.parse(combinedMd);
  logInfo(`markdown → HTML conversion complete (${bodyHtml.length} bytes)`);

  // Pre-render mermaid, write SVGs, swap placeholders for .svg-figure wrappers.
  let mermaidCount = 0;
  if (mermaidSources.length > 0 && !mermaidOpts.skipRender) {
    logInfo(`rendering ${mermaidSources.length} mermaid diagrams via headless puppeteer...`);
    const svgs = await renderMermaidSources(mermaidSources, { theme: mermaidOpts.theme });
    mkdirSync(join(OUTPUT_DIR, 'assets', 'svg'), { recursive: true });
    for (let i = 0; i < svgs.length; i += 1) {
      const fileName = `mermaid-${String(i + 1).padStart(2, '0')}.svg`;
      writeFileSync(join(OUTPUT_DIR, 'assets', 'svg', fileName), svgs[i]);
      const figure =
        `<figure class="svg-figure" tabindex="0" aria-label="Diagram ${i + 1}">` +
        `<img src="assets/svg/${fileName}" alt="Diagram ${i + 1}" />` +
        `</figure>`;
      bodyHtml = bodyHtml.replace(
        new RegExp(`(?:<p>\\s*)?@@MERMAID_PLACEHOLDER_${i}@@(?:\\s*</p>)?`),
        figure,
      );
      mermaidCount += 1;
    }
    logInfo(`pre-rendered ${mermaidCount} mermaid diagrams`);
  }

  // Heading-id resolution:
  //   1. Attach explicit <!-- .slide: id="X" --> pins to matching headings.
  //   2. Auto-slug every other heading; deck-wide uniqueness via -2, -3 suffixes.
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
  // Prime slug table with explicit pins so auto-slugging can't collide with them.
  for (const pin of explicitPins) seenSlugs.set(pin.id, 1);

  // Idempotent HTML tag-strip (CodeQL js/incomplete-multi-character-sanitization
  // safe — loops until stable).
  function stripTags(s) {
    let prev;
    let out = String(s);
    do {
      prev = out;
      out = out.replace(/<[^>]*>/g, '');
    } while (out !== prev);
    return out;
  }

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
        break;
      }
      return match;
    },
  );
  bodyHtml = bodyHtml.replace(
    /<(h[1-6])(?![^>]*\sid=)>([\s\S]*?)<\/\1>/g,
    (match, tag, inner) => {
      const plain = stripTags(inner).trim();
      const id = slugify(plain);
      return id ? `<${tag} id="${id}">${inner}</${tag}>` : match;
    },
  );

  // Assemble index.html shell.
  const indexHtml = `<!doctype html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>${title}</title>
  <link rel="icon" href="favicon.ico" />
  <link rel="stylesheet" href="assets/css/zenburn.min.css" />
  <link rel="stylesheet" href="assets/css/slides-overrides.css" />
</head>
<body>
  <main class="deck">
${bodyHtml}
  </main>
  <!-- Mermaid is pre-rendered to SVG at build time; the viewer handles -->
  <!-- pan/zoom/fullscreen for every diagram.                            -->
  <script defer src="assets/js/svg-viewer.js"></script>
</body>
</html>
`;

  writeFileSync(join(OUTPUT_DIR, 'index.html'), indexHtml);
  logInfo(`wrote index.html (${indexHtml.length} bytes)`);

  // Copy aux assets into the output package.
  mkdirSync(join(OUTPUT_DIR, 'assets', 'css'), { recursive: true });
  mkdirSync(join(OUTPUT_DIR, 'assets', 'js'), { recursive: true });

  cpSync(CSS_SRC, join(OUTPUT_DIR, 'assets', 'css', 'slides-overrides.css'));
  logInfo('bundled assets/css/slides-overrides.css');

  cpSync(FRAMEWORK_DEFAULTS.highlightStyle, join(OUTPUT_DIR, 'assets', 'css', 'zenburn.min.css'));
  logInfo('bundled assets/css/zenburn.min.css');

  cpSync(SVG_VIEWER_SRC, join(OUTPUT_DIR, 'assets', 'js', 'svg-viewer.js'));
  logInfo(`bundled assets/js/svg-viewer.js (${statSync(SVG_VIEWER_SRC).size} bytes)`);

  // Hand-authored SVGs land beside the pre-rendered mermaid-*.svg. Merge:
  // cpSync of the user's assets dir onto the already-populated staging dir;
  // the user's hand-authored assets layer on top of generated artifacts (same
  // filename collides deterministically).
  if (ASSETS_DIR && existsSync(ASSETS_DIR)) {
    cpSync(ASSETS_DIR, join(OUTPUT_DIR, 'assets'), { recursive: true });
    logInfo(`bundled hand-authored assets from ${ASSETS_DIR}`);
  }

  if (existsSync(ICON_SRC)) {
    cpSync(ICON_SRC, join(OUTPUT_DIR, 'favicon.ico'));
  }

  // Drift breadcrumb — SHA of the deck + each override file. Lets consumers
  // CI-gate "did someone edit the deck without re-syncing the embed?"
  const deckSha = createHash('sha256').update(readFileSync(DECK_SRC)).digest('hex');
  const cssSha = createHash('sha256').update(readFileSync(CSS_SRC)).digest('hex');
  const viewerSha = createHash('sha256').update(readFileSync(SVG_VIEWER_SRC)).digest('hex');
  writeFileSync(
    join(OUTPUT_DIR, 'SOURCE_SHA256.txt'),
    [
      '# tutorial-forge drift breadcrumb',
      '# consumer CI gate can hash the configured input files and compare.',
      `deck_sha256=${deckSha}`,
      `css_sha256=${cssSha}`,
      `svg_viewer_sha256=${viewerSha}`,
      `deck_path=${config.deck}`,
      `css_path=${config.cssPath ?? '<framework default>'}`,
      `svg_viewer_path=${config.svgViewerPath ?? '<framework default>'}`,
      '',
    ].join('\n'),
  );
  logInfo(
    `wrote SOURCE_SHA256.txt (deck=${deckSha.slice(0, 12)}... css=${cssSha.slice(0, 12)}... viewer=${viewerSha.slice(0, 12)}...)`,
  );

  const [fileCount, totalBytes] = countFiles(OUTPUT_DIR);
  logInfo(`DONE: ${fileCount} files, ${(totalBytes / (1024 * 1024)).toFixed(2)} MB total`);

  return {
    files: fileCount,
    bytes: totalBytes,
    mermaidCount,
    acronymsApplied,
    deckSha,
    cssSha,
    viewerSha,
  };
}

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
