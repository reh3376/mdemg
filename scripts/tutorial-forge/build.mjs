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

  // Acronym transform:
  //   * First N occurrences get rewritten "Expansion (ACRONYM)" (idempotent).
  //   * Every subsequent occurrence becomes a link to the glossary entry.
  //   * Glossary is generated below from acronyms that actually appeared.
  const acronyms = loadAcronyms(acronymsOpts);
  const glossaryEnabled = (acronymsOpts.glossary ?? {}).enabled !== false;
  const glossaryIdFor = (acr) => `glossary-${acr.toLowerCase()}`;
  const linkGenerator = glossaryEnabled
    ? (acr) => `<a class="acronym-link" href="#${glossaryIdFor(acr)}">${acr}</a>`
    : null;
  const {
    text: afterAcronyms,
    applied: acronymsApplied,
    linked: acronymsLinked,
    used: usedAcronyms,
  } = expandAcronyms(md, acronyms, expandCount, { linkGenerator });
  md = afterAcronyms;
  const appliedCount = Object.values(acronymsApplied).reduce((a, b) => a + b, 0);
  const linkedCount = Object.values(acronymsLinked).reduce((a, b) => a + b, 0);
  if (appliedCount > 0 || linkedCount > 0) {
    logInfo(
      `acronyms: expanded ${appliedCount}, linked ${linkedCount}, used ${usedAcronyms.length} distinct terms`,
    );
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

  // Build the glossary section from acronyms that appear in the deck. We emit
  // raw HTML directly (not markdown) so the acronym transform never sees it
  // and can't try to re-link acronyms inside their own definitions.
  if (glossaryEnabled && usedAcronyms.length > 0) {
    const glossaryLabel = (acronymsOpts.glossary ?? {}).label ?? 'Glossary';
    const items = usedAcronyms
      .map((acr) => {
        const id = glossaryIdFor(acr);
        const expansion = acronyms[acr];
        return `  <div class="glossary-entry">\n    <dt id="${id}">${escapeHtml(acr)}</dt>\n    <dd>${escapeHtml(expansion)}</dd>\n  </div>`;
      })
      .join('\n');
    bodyHtml += `
<section class="glossary" id="glossary">
  <h1 id="glossary-top">${escapeHtml(glossaryLabel)}</h1>
  <p class="glossary-intro">Every acronym used in this tutorial, with its expansion. Each acronym occurrence after the first in the body text is a link to its entry here. Use the floating <strong>← Return to Previous</strong> button at the top-right of the viewport to jump back to where you were reading.</p>
  <dl class="glossary-list">
${items}
  </dl>
</section>
`;
    logInfo(`built glossary section with ${usedAcronyms.length} entries`);
  }

  // Pre-render mermaid, write SVGs, swap placeholders for .svg-figure wrappers.
  // Each .svg-figure gets a stable id="diagram-NN" so the Graphs TOC can link
  // directly to it.
  let mermaidCount = 0;
  if (mermaidSources.length > 0 && !mermaidOpts.skipRender) {
    logInfo(`rendering ${mermaidSources.length} mermaid diagrams via headless puppeteer...`);
    const svgs = await renderMermaidSources(mermaidSources, { theme: mermaidOpts.theme });
    mkdirSync(join(OUTPUT_DIR, 'assets', 'svg'), { recursive: true });
    for (let i = 0; i < svgs.length; i += 1) {
      const fileName = `mermaid-${String(i + 1).padStart(2, '0')}.svg`;
      const diagramId = `diagram-${String(i + 1).padStart(2, '0')}`;
      writeFileSync(join(OUTPUT_DIR, 'assets', 'svg', fileName), svgs[i]);
      const figure =
        `<figure id="${diagramId}" class="svg-figure" tabindex="0" aria-label="Diagram ${i + 1}">` +
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

  function escapeHtml(s) {
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

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

  // Build the Table of Contents from headings in bodyHtml. The sidebar is
  // persistent (CSS-fixed at the left margin) so a reader can jump to any
  // section without scrolling. Default: H1+H2; configurable per repo.
  const tocOpts = config.toc ?? {};
  const tocEnabled = tocOpts.enabled !== false;
  const tocMinLevel = tocOpts.minLevel ?? 1;
  const tocMaxLevel = tocOpts.maxLevel ?? 2;
  const tocLabel = tocOpts.label ?? 'Contents';

  let tocHtml = '';
  let tocEntryCount = 0;
  let graphsEntryCount = 0;
  if (tocEnabled) {
    // Pass A: collect (level, id, text, pos) for every heading. We keep the
    // byte-position so we can later associate each .svg-figure with the
    // nearest preceding heading as its human-readable label.
    const allHeadings = [];
    const headingRe = /<(h([1-6]))[^>]*\sid="([^"]+)"[^>]*>([\s\S]*?)<\/h\2>/g;
    for (const m of bodyHtml.matchAll(headingRe)) {
      allHeadings.push({
        level: Number(m[2]),
        id: m[3],
        text: stripTags(m[4]).replace(/\s+/g, ' ').trim(),
        pos: m.index,
      });
    }

    // Sections entries: headings whose level is in config range.
    const sectionEntries = allHeadings.filter(
      (h) => h.level >= tocMinLevel && h.level <= tocMaxLevel && h.text,
    );
    tocEntryCount = sectionEntries.length;

    // Graphs entries: every <figure class="svg-figure">, whether it already has
    // an id (pre-rendered mermaid gets diagram-NN) or not (hand-authored HTML
    // that didn't set an id — the build auto-assigns one). Each entry is
    // labeled with its nearest preceding heading for human readability.
    const graphsEnabled = (tocOpts.graphs ?? {}).enabled !== false;
    const graphEntries = [];
    if (graphsEnabled) {
      // Inject an id onto any .svg-figure that lacks one. Pre-rendered mermaid
      // diagrams already have id="diagram-NN"; hand-authored HTML without an
      // id gets figure-NN appended in document order.
      let synth = 0;
      bodyHtml = bodyHtml.replace(
        /<figure\b([^>]*)>/g,
        (full, attrs) => {
          if (!/class="svg-figure"/.test(attrs)) return full;
          if (/\sid="/.test(attrs)) return full;
          synth += 1;
          const id = `figure-${String(synth).padStart(2, '0')}`;
          return `<figure id="${id}"${attrs}>`;
        },
      );
      // Enumerate every .svg-figure (now all have ids) in document order.
      const figureRe = /<figure\s+id="([^"]+)"[^>]*class="svg-figure"[^>]*>/g;
      const headingsByPos = allHeadings.slice().sort((a, b) => a.pos - b.pos);
      for (const m of bodyHtml.matchAll(figureRe)) {
        const figId = m[1];
        const figPos = m.index;
        let nearest = null;
        for (const h of headingsByPos) {
          if (h.pos < figPos) nearest = h;
          else break;
        }
        const label = nearest ? nearest.text : `Figure ${graphEntries.length + 1}`;
        graphEntries.push({ id: figId, label });
      }
      graphsEntryCount = graphEntries.length;
    }

    if (sectionEntries.length > 0 || graphEntries.length > 0) {
      let sectionsHtml = '';
      if (sectionEntries.length > 0) {
        const items = sectionEntries
          .map(
            (e) =>
              `      <li class="deck-toc-l${e.level}"><a href="#${e.id}">${escapeHtml(e.text)}</a></li>`,
          )
          .join('\n');
        sectionsHtml = `    <ul class="deck-toc-sections">
${items}
    </ul>
`;
      }
      let graphsHtml = '';
      if (graphEntries.length > 0) {
        const graphsLabel = (tocOpts.graphs ?? {}).label ?? 'Graphs';
        const items = graphEntries
          .map(
            (e, i) =>
              `      <li class="deck-toc-graph"><a href="#${e.id}">${String(i + 1).padStart(2, '0')}. ${escapeHtml(e.label)}</a></li>`,
          )
          .join('\n');
        graphsHtml = `    <div class="deck-toc-subhead">${escapeHtml(graphsLabel)}</div>
    <ul class="deck-toc-graphs">
${items}
    </ul>
`;
      }
      tocHtml = `  <nav class="deck-toc" aria-label="${escapeHtml(tocLabel)}">
    <div class="deck-toc-head">${escapeHtml(tocLabel)}</div>
${sectionsHtml}${graphsHtml}  </nav>
`;
      logInfo(
        `built TOC with ${sectionEntries.length} section entries (H${tocMinLevel}–H${tocMaxLevel}) + ${graphEntries.length} graph entries`,
      );
    }
  }

  // Scroll-sync IIFE: observe heading intersection and highlight the matching
  // TOC entry as the user scrolls. Zero deps; graceful no-op when no TOC.
  const scrollSyncScript = tocHtml
    ? `  <script>
    (function () {
      const links = document.querySelectorAll('.deck-toc a[href^="#"]');
      if (!links.length) return;
      const map = new Map();
      for (const a of links) map.set(a.getAttribute('href').slice(1), a);
      const targets = Array.from(map.keys())
        .map((id) => document.getElementById(id))
        .filter(Boolean);
      if (!targets.length) return;
      let active = null;
      function setActive(id) {
        const a = map.get(id);
        if (!a || a === active) return;
        if (active) active.classList.remove('deck-toc-active');
        a.classList.add('deck-toc-active');
        active = a;
        a.scrollIntoView({ block: 'nearest', inline: 'nearest' });
      }
      const observer = new IntersectionObserver(
        (entries) => {
          const visible = entries.filter((e) => e.isIntersecting);
          if (!visible.length) return;
          const top = visible.reduce((a, b) =>
            a.boundingClientRect.top < b.boundingClientRect.top ? a : b,
          );
          setActive(top.target.id);
        },
        { rootMargin: '0px 0px -70% 0px', threshold: 0 },
      );
      for (const t of targets) observer.observe(t);
    })();
  </script>
`
    : '';

  // Assemble index.html shell.
  const indexHtml = `<!doctype html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>${escapeHtml(title)}</title>
  <link rel="icon" href="favicon.ico" />
  <link rel="stylesheet" href="assets/css/zenburn.min.css" />
  <link rel="stylesheet" href="assets/css/slides-overrides.css" />
</head>
<body${tocHtml ? ' class="has-toc"' : ''}>
${tocHtml}  <main class="deck">
${bodyHtml}
  </main>
  <!-- Floating "Return to Previous" button. Visible only when the current -->
  <!-- hash points at a glossary entry; clicking fires history.back().     -->
  <button
    id="glossary-back-btn"
    class="glossary-back-btn"
    type="button"
    aria-label="Return to Previous location"
    hidden
  >← Return to Previous</button>
  <!-- Mermaid is pre-rendered to SVG at build time; the viewer handles -->
  <!-- pan/zoom/fullscreen for every diagram.                            -->
  <script defer src="assets/js/svg-viewer.js"></script>
  <script>
    // Floating "Return to Previous" button logic.
    //
    // Shows the button whenever the current URL hash points at a glossary
    // entry (e.g. #glossary-llm, #glossary-top). Clicking the button fires
    // window.history.back(), returning the reader to the location they
    // clicked the acronym from. If the reader scrolled to the glossary
    // manually (no history to go back to) the button stays hidden.
    (function () {
      var btn = document.getElementById('glossary-back-btn');
      if (!btn) return;
      function atGlossary() {
        var h = window.location.hash || '';
        return h.indexOf('#glossary') === 0;
      }
      function updateBtn() {
        if (atGlossary() && window.history.length > 1) {
          btn.hidden = false;
        } else {
          btn.hidden = true;
        }
      }
      btn.addEventListener('click', function () {
        window.history.back();
      });
      window.addEventListener('hashchange', updateBtn);
      // Delay the initial check briefly so hashchange events during page
      // load (reveal.js-era deep-links, scroll-anchor normalization) don't
      // flash the button before the reader is ready.
      setTimeout(updateBtn, 50);
    })();
  </script>
${scrollSyncScript}</body>
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
    tocEntryCount,
    graphsEntryCount,
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
