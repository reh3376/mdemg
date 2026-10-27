// Build-time Mermaid → SVG renderer. Launches a headless Puppeteer page with
// the bundled mermaid.min.js, calls mermaid.render() per source block, and
// returns the resulting SVG strings. Called from export.mjs so the exported
// deck ships hand-authored-shaped SVGs (wrapped in <figure class="svg-figure">
// by export.mjs) instead of client-rendered <div class="mermaid"> blocks.
//
// Why pre-render:
//   * Mermaid-rendered diagrams in the deck looked awkward beside the one
//     hand-authored u4-how-jiminy-thinks.svg (operator feedback 2026-10-27).
//     Rendering them to SVG at build time lets each flow through the shared
//     .svg-figure interactive viewer (toolbar + wheel zoom + drag pan +
//     fullscreen) — the exact treatment that makes Jiminy's diagram feel
//     professional.
//   * Dropping mermaid.min.js + the client-side initialize() shaves ~2.5 MB
//     from the shipped embed and removes a runtime JS dependency.

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer';

const SCRIPT_DIR = dirname(fileURLToPath(import.meta.url));
const MERMAID_SRC = join(SCRIPT_DIR, 'node_modules', 'mermaid', 'dist', 'mermaid.min.js');

/**
 * Render N mermaid source strings to SVG strings. Returns a parallel array.
 * Order of inputs is preserved. Each SVG is self-contained (has its own
 * <svg> root with the viewBox mermaid computed).
 *
 * @param {string[]} sources
 * @returns {Promise<string[]>}
 */
export async function renderMermaidSources(sources) {
  if (sources.length === 0) return [];

  const mermaidJs = readFileSync(MERMAID_SRC, 'utf-8');
  const browser = await puppeteer.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox'],
  });

  try {
    const page = await browser.newPage();

    // Minimal shell. We never navigate — just set content with the mermaid
    // bundle inline so we don't need an HTTP server during build.
    await page.setContent(
      `<!doctype html><html><head><meta charset="utf-8"></head>
       <body><div id="host"></div>
       <script>${mermaidJs}</script>
       <script>
         // Light theme — the .svg-figure wrapper has a white background to
         // match the hand-authored diagrams; mermaid dark-theme output on a
         // white frame would make text barely visible.
         // htmlLabels:true is more permissive on label syntax (unquoted parens,
         // slashes, ampersands) which the deck's existing mermaid blocks rely on.
         // Output embeds <foreignObject> HTML inside the SVG — modern browsers
         // render it correctly and the .svg-figure viewer handles pan/zoom
         // over the whole SVG uniformly.
         window.mermaid.initialize({
           startOnLoad: false,
           theme: 'default',
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
      // ```mermaid blocks were authored for client-side rendering with
      // forgiving defaults; the strict parser needs:
      //
      //   1. <br/> → \n  (mermaid's native newline inside labels)
      //   2. Unquoted labels [like this] containing ( ) / & — wrap in "..."
      //      because the tokenizer treats ( as the start of a stadium shape.
      //      Mermaid's own fix for this class is author-side quoting.
      //
      // We apply (1) first because it introduces \n which later may land next
      // to a (, then (2) wraps the whole label.
      let source = sources[i]
        .replace(/<br\s*\/?>/gi, '\\n')
        .replace(/&lt;br\s*\/?&gt;/gi, '\\n');
      // Auto-quote rectangle-node labels that contain mermaid-special chars.
      // Match [content] where content isn't already quoted and has (, ), /, &,
      // or a `\n`. We skip `[[...]]` subroutine shape (`[[` already escapes).
      source = source.replace(
        /(^|[^[\]"'])\[([^\]"\n]+)\]/g,
        (full, pre, inner) => {
          if (/[()/&]|\\n/.test(inner)) {
            // Escape any embedded " just in case
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
