// Build-time Mermaid → SVG renderer. Launches a headless Puppeteer page with
// the bundled mermaid.min.js, calls mermaid.render() per source block, and
// returns the resulting SVG strings in input order.
//
// Why pre-render instead of client-side rendering:
//   * SVG output ships with the static package — no mermaid.min.js runtime
//     (~2.5 MB saved).
//   * Each pre-rendered SVG becomes an <img> inside the shared .svg-figure
//     interactive viewer (toolbar + wheel-zoom + drag-pan + fullscreen), so
//     all diagrams get a consistent professional presentation.
//
// This module expects mermaid.min.js at <scripts/tutorial-forge/node_modules/
// mermaid/dist/mermaid.min.js>. Framework consumers install the package via
// `npm install` in scripts/tutorial-forge/.

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer';

const PKG_DIR = dirname(dirname(fileURLToPath(import.meta.url)));
const MERMAID_SRC = join(PKG_DIR, 'node_modules', 'mermaid', 'dist', 'mermaid.min.js');

/**
 * Render N mermaid source strings to SVG strings. Order of inputs is preserved.
 * Each SVG is self-contained (has its own <svg> root with the viewBox mermaid
 * computed).
 *
 * @param {string[]} sources
 * @param {object} [opts]
 * @param {string} [opts.theme='default']
 * @returns {Promise<string[]>}
 */
export async function renderMermaidSources(sources, opts = {}) {
  if (sources.length === 0) return [];
  const theme = opts.theme ?? 'default';

  const mermaidJs = readFileSync(MERMAID_SRC, 'utf-8');
  const browser = await puppeteer.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox'],
  });

  try {
    const page = await browser.newPage();
    await page.setContent(
      `<!doctype html><html><head><meta charset="utf-8"></head>
       <body><div id="host"></div>
       <script>${mermaidJs}</script>
       <script>
         // htmlLabels:true is permissive on label syntax (unquoted parens,
         // slashes, ampersands) which real-world deck authors rely on.
         // Output embeds <foreignObject> HTML inside the SVG — modern browsers
         // render it correctly and the .svg-figure viewer handles pan/zoom
         // over the whole SVG uniformly.
         window.mermaid.initialize({
           startOnLoad: false,
           theme: ${JSON.stringify(theme)},
           securityLevel: 'loose',
           flowchart: { useMaxWidth: false, htmlLabels: true, curve: 'basis' },
           sequence: { useMaxWidth: false, boxMargin: 10 },
           gantt: { useMaxWidth: false },
           themeVariables: {
             fontSize: '15px',
             fontFamily: '-apple-system, "Inter", "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
           },
         });
       </script></body></html>`,
      { waitUntil: 'load' },
    );

    const svgs = [];
    for (let i = 0; i < sources.length; i += 1) {
      // Normalize common authoring shapes mermaid rejects. The deck's existing
      // ```mermaid blocks are often authored for forgiving client-side defaults;
      // the strict server-side parser needs:
      //   1. <br/> → \n   (mermaid's native newline inside labels)
      //   2. Unquoted [labels] containing ( ) / & → wrapped in "..."
      //      because the tokenizer treats ( as the start of a stadium shape.
      let source = sources[i]
        .replace(/<br\s*\/?>/gi, '\\n')
        .replace(/&lt;br\s*\/?&gt;/gi, '\\n');
      source = source.replace(
        /(^|[^[\]"'])\[([^\]"\n]+)\]/g,
        (full, pre, inner) => {
          if (/[()/&]|\\n/.test(inner)) {
            const safe = inner.replace(/"/g, '&quot;');
            return `${pre}["${safe}"]`;
          }
          return full;
        },
      );

      const result = await page.evaluate(
        async ({ id, src }) => {
          try {
            const { svg } = await window.mermaid.render(id, src);
            return { ok: true, svg };
          } catch (err) {
            return { ok: false, err: String(err?.message || err) };
          }
        },
        { id: `mermaid-render-${i}`, src: source },
      );
      if (!result.ok) {
        throw new Error(
          `mermaid render failed for block ${i + 1}: ${result.err}\n--- source ---\n${source}\n--- end ---`,
        );
      }
      svgs.push(result.svg);
    }
    return svgs;
  } finally {
    await browser.close();
  }
}
