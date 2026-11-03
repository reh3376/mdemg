// Framework tests: node --test (no Chromium; Mermaid rendering is skipped).
// They hold the heading-id and TOC-label semantics documented in FSD §7.1
// and §15.2, and the framework favicon default of §4.5.

import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { after, before, describe, it } from 'node:test';

import { build } from '../build.mjs';

const PKG_DIR = dirname(dirname(fileURLToPath(import.meta.url)));

// LLM is in defaults/acronyms.json: two expansions, then glossary links, so the
// pinned "An LLM Heading" below renders with a link inside it.
const DECK = `# Tutorial

<!-- .slide: id="cant" -->
## You Grade What The System Can't & More

Body with LLM once, LLM twice, LLM thrice.

---

<!-- .slide: id="llm-heading" -->
## An LLM Heading

Text.

---

<!-- .slide: id="later-pin" -->
## Later Pin

Text.

---

## Q&A

\`\`\`bash
# a comment, not a heading
\`\`\`

---

<!-- .slide: id="only-code" -->
\`\`\`bash
# this section's only '#' line is code
\`\`\`
`;

describe('tutorial-forge build', () => {
  let dir;
  let html;
  let out;

  before(async () => {
    dir = mkdtempSync(join(tmpdir(), 'tutorial-forge-test-'));
    writeFileSync(join(dir, 'deck.md'), DECK);
    out = join(dir, 'dist');
    mkdirSync(out);
    await build(
      { deck: 'deck.md', output: 'dist', title: "Test's & Co", mermaid: { skipRender: true } },
      dir,
    );
    html = readFileSync(join(out, 'index.html'), 'utf-8');
  });

  after(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it('pins a heading whose text has an apostrophe and an ampersand', () => {
    assert.match(html, /<h2 id="cant">You Grade What The System Can&#39;t &amp; More<\/h2>/);
  });

  it('pins a heading that carries a glossary link', () => {
    assert.match(html, /<h2 id="llm-heading">An <a class="acronym-link"[^>]*>LLM<\/a> Heading<\/h2>/);
  });

  it('attaches a later pin even when earlier headings were hard to match', () => {
    assert.match(html, /<h2 id="later-pin">Later Pin<\/h2>/);
  });

  it('drops a pin whose section has no heading outside fenced code', () => {
    assert.doesNotMatch(html, /only-code/);
    assert.match(html, /# this section&#39;s only &#39;#&#39; line is code/);
  });

  it('leaves no pin marker in the page', () => {
    assert.doesNotMatch(html, /tf-pin/);
  });

  it('slugs an unpinned heading from its decoded text', () => {
    assert.match(html, /<h2 id="qa">Q&amp;A<\/h2>/);
    assert.match(html, /<h1 id="tutorial">Tutorial<\/h1>/);
  });

  it('escapes a sidebar label exactly once', () => {
    assert.match(html, /<a href="#cant">You Grade What The System Can&#39;t &amp; More<\/a>/);
    assert.doesNotMatch(html, /&amp;#39;|&amp;amp;/);
  });

  it('escapes the title once', () => {
    assert.match(html, /<title>Test&#39;s &amp; Co<\/title>/);
  });

  it('ships the framework favicon from templates/', () => {
    const shipped = readFileSync(join(out, 'favicon.ico'));
    const source = readFileSync(join(PKG_DIR, 'templates', 'favicon.ico'));
    assert.ok(shipped.equals(source));
  });
});
