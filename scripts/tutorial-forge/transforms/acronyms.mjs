// Acronym transform. Walks the markdown deck line by line (preserving fenced
// code blocks, HTML tags, inline code, and markdown-link URLs untouched) and
// expands the first N occurrences of each known acronym to "Expansion (ACRONYM)".
//
// The acronym dictionary comes from (merged left-to-right, right wins):
//   1. defaults/acronyms.json — industry-standard terms
//   2. config.acronyms.extra  — per-repo additions and overrides
//   3. config.acronyms.disabled — list of keys to drop (removes default
//      entries entirely so project-internal acronyms stay untouched)

import { readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const PKG_DIR = dirname(dirname(fileURLToPath(import.meta.url)));
const DEFAULTS_PATH = join(PKG_DIR, 'defaults', 'acronyms.json');

export function loadAcronyms(configAcronyms = {}) {
  const defaults = JSON.parse(readFileSync(DEFAULTS_PATH, 'utf-8'));
  // Drop the comment key.
  delete defaults._comment;
  const extra = configAcronyms.extra ?? {};
  const disabled = new Set(configAcronyms.disabled ?? []);
  const merged = { ...defaults, ...extra };
  for (const key of disabled) delete merged[key];
  return merged;
}

/**
 * Expand or link each acronym based on its position in the document.
 *
 * The first `expandCount` occurrences are rewritten to `Expansion (ACRONYM)`.
 * Every subsequent occurrence is wrapped by `linkGenerator(acronym)` if one
 * is passed (default: no link, bare acronym). This supports the "define
 * once, link to glossary afterward" UX.
 *
 * @param {string} deckText         raw markdown
 * @param {object} acronyms         map ACRONYM → expansion
 * @param {number} expandCount      how many times each acronym gets expanded (default 1)
 * @param {object} [opts]
 * @param {function(string):string} [opts.linkGenerator]  called for the Nth+1
 *   occurrence of each acronym and beyond; returns the replacement HTML.
 *   When omitted or returning falsy, the acronym stays bare.
 * @returns {{text: string, applied: Record<string,number>, linked: Record<string,number>, used: string[]}}
 */
export function expandAcronyms(deckText, acronyms, expandCount = 1, opts = {}) {
  const counts = Object.fromEntries(Object.keys(acronyms).map((k) => [k, 0]));
  const linkCounts = Object.fromEntries(Object.keys(acronyms).map((k) => [k, 0]));
  const linkGenerator = typeof opts.linkGenerator === 'function' ? opts.linkGenerator : null;
  if (Object.keys(acronyms).length === 0) {
    return { text: deckText, applied: counts, linked: linkCounts, used: [] };
  }
  // Longest-first so "UVTS" doesn't get eaten by "UV" if "UV" were a key.
  const keys = Object.keys(acronyms).sort((a, b) => b.length - a.length);
  const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const acronymRe = new RegExp(
    `(?<!\\w)(${keys.map(escapeRe).join('|')})(?!\\w)`,
    'g',
  );

  // Idempotency pre-pass: for each acronym, count how many existing
  // definitions already appear in the deck in either canonical format:
  //   * "Expansion (ACRONYM)" — the standard inline form.
  //   * "ACRONYM: Expansion"  — the title/heading form (e.g. "# MDEMG: ...").
  // Those count toward the per-acronym budget — if the deck already defines
  // LLM twice via either form, we skip further expansion. The transform
  // remains idempotent across repeated builds.
  const prebudgetUsed = {};
  for (const key of keys) {
    const expansion = acronyms[key];
    const expandedParenRe = new RegExp(`${escapeRe(expansion)} \\(${escapeRe(key)}\\)`, 'g');
    const titleFormRe = new RegExp(`\\b${escapeRe(key)}:\\s+${escapeRe(expansion)}\\b`, 'g');
    const already =
      (deckText.match(expandedParenRe) ?? []).length +
      (deckText.match(titleFormRe) ?? []).length;
    prebudgetUsed[key] = already;
    counts[key] = already;
  }

  const outLines = [];
  let inFence = false;
  for (const rawLine of deckText.split(/(?<=\n)/)) {
    const line = rawLine;
    const stripped = line.trim();
    // Fenced-code toggle (``` on its own line).
    if (stripped.startsWith('```')) {
      inFence = !inFence;
      outLines.push(line);
      continue;
    }
    if (inFence) {
      outLines.push(line);
      continue;
    }
    // Pure HTML-comment lines (e.g. slide directives): skip.
    if (stripped.startsWith('<!--') && stripped.endsWith('-->')) {
      outLines.push(line);
      continue;
    }
    // Build a scan-mask: for each inline-code span, HTML tag, or markdown-link
    // URL, blank out the content so acronym matches never land inside.
    const masks = [];
    for (const m of line.matchAll(/`[^`\n]+`/g)) {
      masks.push([m.index, m.index + m[0].length]);
    }
    for (const m of line.matchAll(/<[^>\n]+>/g)) {
      masks.push([m.index, m.index + m[0].length]);
    }
    for (const m of line.matchAll(/\]\([^)\n]+\)/g)) {
      masks.push([m.index, m.index + m[0].length]);
    }
    masks.sort((a, b) => a[0] - b[0]);
    const merged = [];
    for (const [s, e] of masks) {
      if (merged.length && s < merged[merged.length - 1][1]) continue;
      merged.push([s, e]);
    }
    const scanChars = line.split('');
    for (const [s, e] of merged) for (let i = s; i < e; i += 1) scanChars[i] = ' ';
    const scan = scanChars.join('');

    const edits = [];
    for (const m of scan.matchAll(acronymRe)) {
      const tok = m[1];
      const start = m.index;
      // Already-defined form "Expansion (ACRONYM)": skip the ACRONYM inside
      // the parenthesized suffix.
      if (start > 0 && line[start - 1] === '(') continue;
      // Title-form definition "ACRONYM: Expansion": the acronym IS the
      // definition's subject; don't rewrite or link it, otherwise the title
      // becomes a link to itself (or worse, "Expansion (ACRONYM): Expansion").
      const after = line.slice(start + tok.length);
      const titleFormCheck = new RegExp(`^:\\s+${escapeRe(acronyms[tok])}\\b`);
      if (titleFormCheck.test(after)) continue;
      if (counts[tok] < expandCount) {
        counts[tok] += 1;
        edits.push([start, start + tok.length, `${acronyms[tok]} (${tok})`]);
      } else if (linkGenerator) {
        const linkHtml = linkGenerator(tok);
        if (linkHtml) {
          linkCounts[tok] += 1;
          edits.push([start, start + tok.length, linkHtml]);
        }
      }
    }
    if (edits.length === 0) {
      outLines.push(line);
      continue;
    }
    let edited = line;
    for (let i = edits.length - 1; i >= 0; i -= 1) {
      const [s, e, rep] = edits[i];
      edited = edited.slice(0, s) + rep + edited.slice(e);
    }
    outLines.push(edited);
  }
  // Report only NEW expansions this pass, not the pre-existing ones that
  // counted toward the budget.
  const applied = {};
  for (const [k, v] of Object.entries(counts)) {
    const net = v - (prebudgetUsed[k] ?? 0);
    if (net > 0) applied[k] = net;
  }
  const linked = {};
  for (const [k, v] of Object.entries(linkCounts)) {
    if (v > 0) linked[k] = v;
  }
  // Set of acronyms that showed up at least once (expanded OR linked OR
  // already-present as an "Expansion (ACRONYM)" form) — this drives the
  // glossary section downstream.
  const used = new Set();
  for (const k of Object.keys(applied)) used.add(k);
  for (const k of Object.keys(linked)) used.add(k);
  for (const [k, v] of Object.entries(prebudgetUsed)) if (v > 0) used.add(k);
  return { text: outLines.join(''), applied, linked, used: Array.from(used).sort() };
}

/**
 * Standalone CLI helper for ad-hoc use (same as scripts/tutorial/expand_acronyms.py
 * used to be). Resolves paths against process.cwd().
 */
export function expandAcronymsInFile(deckPath, acronyms, expandCount = 2) {
  const absPath = resolve(process.cwd(), deckPath);
  const original = readFileSync(absPath, 'utf-8');
  const { text, applied } = expandAcronyms(original, acronyms, expandCount);
  return { changed: text !== original, text, applied, path: absPath };
}
