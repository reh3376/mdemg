// rules.js — JIMINY-RULES-UI-001 Epic 4 (UX-review revision, 2026-08-13)
//
// Interaction model (post-operator UX review):
//   - Row click = expand/collapse INLINE (accordion pattern; details render
//     as a row directly BELOW the clicked row, not at the bottom of the page)
//   - Expanded row shows: metadata + content + recent outcomes + [Edit] +
//     [Tombstone] + [Close] buttons
//   - Edit → shows an inline form in the SAME expanded row (dropdowns +
//     content textarea + [Save] + [Cancel])
//   - Save (edit) semantics = Option A "supersede" (round-1 immutable
//     lock preserved): 2-call sequence, client-side —
//       (1) POST /v1/jiminy/rules/{code}/tombstone with
//           reason="ui_edit_supersede_<timestamp>"
//       (2) POST /v1/jiminy/rules with same constraint_code + new content
//     Old node stays archived (reversible via un-tombstone); new node is
//     the live version. fetchRuleByCode prefers live (Cypher ORDER BY).
//   - Tombstone → prompt() for reason + confirm dialog (unchanged)
//
// WRITE endpoints are flag-gated server-side by JiminyRulesUIWriteEnabled.

import * as api from '../api.js';
import * as state from '../state.js';
import { h, sectionHeader, clear, helpPanel } from '../utils/dom.js';

const SCOPE_FAMILIES = [
    'git', 'file_mutation', 'bash', 'schema', 'identifier',
    'testing', 'process_docs', 'llm_config', 'cms',
];

let container;
let currentFilters = {};
let searchQuery = '';        // client-side filter on node_id | constraint_code | content
// UX-review fix (2026-08-13): key expansion by node_id (CUIDv2, guaranteed
// unique per rule) NOT constraint_code — duplicates share codes and would
// otherwise all expand together. Live-caught: operator saw two near-dups
// (z5xgcm… + pwa2lm…) both expand from one click.
let expandedNodeID = null;   // node_id of the row currently expanded inline
let expandedMode = 'view';   // 'view' | 'edit'
let showAddForm = false;

export function render(el) {
    container = el;
    load();
}

async function load() {
    const spaceID = state.get('selectedSpace') || 'mdemg-dev';
    try {
        const listResp = await api.rulesList(spaceID, { ...currentFilters, limit: 200 });
        let items = (listResp.data && listResp.data.items) || [];
        // Client-side search: matches node_id (unique per rule) OR
        // constraint_code (human mnemonic) OR content (partial). Case-
        // insensitive. Fires on every keystroke via load().
        if (searchQuery) {
            const q = searchQuery.toLowerCase();
            items = items.filter(it =>
                (it.node_id || '').toLowerCase().includes(q) ||
                (it.constraint_code || '').toLowerCase().includes(q) ||
                (it.content || '').toLowerCase().includes(q));
        }
        // Detail = the SPECIFIC item from the list (unique by node_id) +
        // per-code outcomes. Do NOT round-trip to /v1/jiminy/rules/{code}
        // for metadata — that endpoint disambiguates duplicates via
        // ORDER BY live-preferred so clicking a specific duplicate row
        // would render the OTHER duplicate's data. The list item carries
        // every field the detail metadata needs; only outcomes need a
        // separate call.
        let detail = null;
        if (expandedNodeID) {
            const item = items.find(it => it.node_id === expandedNodeID);
            if (item) {
                // Outcomes are TSDB-per-code — duplicates share the same
                // constraint_code and therefore share their outcome
                // history. That's a data-layer characteristic, not a UI
                // bug (JIMINY-CORRECTION-CORPUS-001 tombstoning is the
                // long-term fix for actual duplicates).
                let outcomes = [];
                let lookbackHours = 168;
                let outcomesWarning = null;
                if (item.constraint_code) {
                    try {
                        const dResp = await api.rulesDetail(item.constraint_code, spaceID);
                        outcomes = (dResp.data && dResp.data.recent_outcomes) || [];
                        lookbackHours = (dResp.data && dResp.data.outcomes_lookback_hours) || 168;
                    } catch (err) {
                        outcomesWarning = err.message;
                    }
                }
                detail = {
                    rule: item,
                    recent_outcomes: outcomes,
                    outcomes_lookback_hours: lookbackHours,
                    outcomes_warning: outcomesWarning,
                };
            } else {
                // Row was filtered/removed — collapse.
                expandedNodeID = null;
            }
        }
        renderTab(items, detail, spaceID);
    } catch (err) {
        clear(container, h('p', { className: 'error' }, `Failed to load rules: ${err.message}`));
    }
}

function renderTab(items, detail, spaceID) {
    const panels = [renderToolbar(spaceID, items.length)];
    if (showAddForm) panels.push(renderAddForm(spaceID));
    panels.push(renderListTable(items, detail, spaceID));
    panels.push(renderHelpPanel());
    clear(container, ...panels);
}

// ─── Toolbar (filters + Add button) ────────────────────────────────────────

function renderToolbar(spaceID, count) {
    const mkSelect = (key, label, opts) => {
        const sel = h('select', {
            className: 'select',
            onchange: (e) => {
                if (e.target.value) currentFilters[key] = e.target.value;
                else delete currentFilters[key];
                expandedNodeID = null; // reset expansion on filter change
                load();
            },
        });
        sel.appendChild(h('option', { value: '' }, `— ${label} —`));
        for (const o of opts) {
            const opt = h('option', { value: o.value }, o.label);
            if (currentFilters[key] === o.value) opt.selected = true;
            sel.appendChild(opt);
        }
        return sel;
    };
    const filterEls = [
        mkSelect('type', 'Type', [
            { value: 'constraint', label: 'constraint' },
            { value: 'correction', label: 'correction' },
        ]),
        mkSelect('severity', 'Severity', [
            { value: 'must', label: 'must' },
            { value: 'must_not', label: 'must_not' },
            { value: 'should', label: 'should' },
            { value: 'note', label: 'note' },
        ]),
        mkSelect('category', 'Category', [
            { value: 'actionable', label: 'actionable' },
            { value: 'informational', label: 'informational' },
        ]),
        mkSelect('scope', 'Scope', SCOPE_FAMILIES.map(s => ({ value: s, label: s }))),
    ];
    const archivedCheck = h('label', { className: 'small' },
        h('input', {
            type: 'checkbox',
            checked: !!currentFilters.include_archived,
            onchange: (e) => {
                if (e.target.checked) currentFilters.include_archived = true;
                else delete currentFilters.include_archived;
                load();
            },
        }),
        ' include archived',
    );
    const addBtn = h('button', {
        className: 'btn btn-primary',
        onclick: () => { showAddForm = !showAddForm; load(); },
    }, showAddForm ? 'Cancel Add' : '+ Add Rule');

    // Search: client-side filter on node_id | constraint_code | content.
    // Every rule has a unique node_id (CUIDv2) that also serves as a
    // searchable stable identifier — paste one to jump to that rule.
    const searchInput = h('input', {
        type: 'text',
        className: 'input',
        placeholder: 'Search: node_id / code / content substring',
        value: searchQuery,
        style: 'min-width: 320px;',
    });
    // Debounce keystrokes so we don't re-render on every character
    let searchTimer = null;
    searchInput.oninput = (e) => {
        clearTimeout(searchTimer);
        const v = e.target.value;
        searchTimer = setTimeout(() => {
            searchQuery = v;
            load();
        }, 200);
    };
    const clearSearchBtn = searchQuery
        ? h('button', { className: 'btn', onclick: () => { searchQuery = ''; load(); } }, '✕ Clear')
        : null;

    return h('div', {},
        sectionHeader(`Jiminy Rules — space=${spaceID}`),
        h('p', { className: 'muted small' },
            `${count} rule${count === 1 ? '' : 's'} shown${searchQuery ? ` (filtered by "${searchQuery}")` : ''}. Click any row to expand it inline. WRITE ops (Add / Edit / Tombstone) are `,
            h('b', {}, 'flag-gated'),
            ' by JIMINY_RULES_UI_WRITE_ENABLED (default false through 2026-08-19; flip in .env + restart to enable).',
        ),
        h('div', { className: 'action-row', style: 'gap: 8px; flex-wrap: wrap; align-items: center;' },
            searchInput, clearSearchBtn, ...filterEls, archivedCheck, addBtn,
        ),
    );
}

// ─── List table (with inline accordion expansion) ──────────────────────────

function renderListTable(items, detail, spaceID) {
    if (items.length === 0) {
        return h('div', {},
            h('p', { className: 'muted' }, 'No rules match the current filters. Clear filters or ',
                h('code', {}, 'include archived'), ' to see more.'),
        );
    }
    const headerRow = h('tr', {},
        h('th', {}, 'Code'),
        h('th', { style: 'font-family: monospace; font-size: 0.85em;' }, 'Node ID'),
        h('th', {}, 'Type'),
        h('th', {}, 'Severity'),
        h('th', {}, 'Category'),
        h('th', {}, 'Scope'),
        h('th', {}, 'Content (head)'),
        h('th', {}, 'Archived'),
        h('th', { style: 'width: 20px;' }, ''),
    );
    const rows = [];
    for (const it of items) {
        // Keyed by node_id (unique CUIDv2 per rule) — NOT constraint_code —
        // so a click on one duplicate doesn't cascade-expand its twins.
        const isExpanded = it.node_id === expandedNodeID;
        const chevron = isExpanded ? '▼' : '▶';
        // Node ID cell — full CUIDv2 in a tooltip; visible portion is
        // click-to-copy-friendly (short-form). Serves as the unique
        // searchable ID per operator directive 2026-08-13.
        const nodeIDCell = h('td', {
            className: 'config-key small',
            style: 'font-family: monospace; cursor: text;',
            title: `Full node_id: ${it.node_id || '—'} (click to expand row; use search box to jump to this exact ID)`,
            onclick: (e) => { e.stopPropagation(); }, // don't trigger row expand when selecting text
        }, (it.node_id || '—').slice(0, 12) + ((it.node_id && it.node_id.length > 12) ? '…' : ''));
        rows.push(h('tr', {
            className: isExpanded ? 'row-selected' : '',
            style: 'cursor: pointer;',
            onclick: () => toggleExpand(it.node_id),
        },
            h('td', { className: 'config-key' }, it.constraint_code || '—'),
            nodeIDCell,
            h('td', {}, it.role_type),
            h('td', {}, it.constraint_type || '—'),
            h('td', {}, it.is_informational ? h('span', { className: 'badge badge-warn' }, 'informational') : 'actionable'),
            h('td', { className: 'small' }, it.scope || '—'),
            h('td', { className: 'small' }, (it.content || '').slice(0, 100) + (it.content && it.content.length > 100 ? '…' : '')),
            h('td', { className: 'small muted' }, it.is_archived ? h('span', { className: 'badge badge-default' }, 'archived') : ''),
            h('td', { className: 'muted' }, chevron),
        ));
        // Inline accordion — details render as an extra row IMMEDIATELY below the clicked row
        if (isExpanded && detail) {
            rows.push(h('tr', {},
                h('td', {
                    colspan: 9,
                    // UX-review fix (2026-08-13): use Catppuccin theme vars,
                    // not light-theme fallbacks. Dark surface + light text.
                    style: 'background: var(--surface0); color: var(--text); padding: 12px; border-top: 2px solid var(--surface1);',
                },
                    renderInlineDetail(detail, spaceID),
                ),
            ));
        }
    }
    return h('div', {},
        h('table', { className: 'config-table' }, h('thead', {}, headerRow), h('tbody', {}, ...rows)),
    );
}

function toggleExpand(nodeID) {
    if (expandedNodeID === nodeID) {
        expandedNodeID = null;
        expandedMode = 'view';
    } else {
        expandedNodeID = nodeID;
        expandedMode = 'view';
    }
    load();
}

// ─── Inline detail (view + edit modes) ─────────────────────────────────────

function renderInlineDetail(data, spaceID) {
    if (data.error) {
        return h('div', {},
            h('p', { className: 'error' }, `Failed to load detail: ${data.error}`),
            h('button', { className: 'btn', onclick: () => { expandedNodeID = null; load(); } }, 'Close'),
        );
    }
    const rule = data.rule || {};
    if (expandedMode === 'edit') return renderEditForm(rule, spaceID);
    return renderViewMode(rule, data);
}

function renderViewMode(rule, data) {
    const outcomes = data.recent_outcomes || [];
    const meta = h('table', { className: 'config-table', style: 'margin-bottom: 8px;' },
        h('tbody', {},
            h('tr', {}, h('td', {}, 'Node ID'), h('td', { className: 'small' }, rule.node_id || '—')),
            h('tr', {}, h('td', {}, 'Type / Severity / Category'),
                h('td', {}, `${rule.role_type || '—'} · ${rule.constraint_type || '—'} · ${rule.is_informational ? 'informational' : 'actionable'}`)),
            h('tr', {}, h('td', {}, 'Scope'), h('td', {}, rule.scope || '—')),
            h('tr', {}, h('td', {}, 'Archived'), h('td', {}, rule.is_archived ? `yes — ${rule.archive_reason || ''}` : 'no')),
            h('tr', {}, h('td', {}, 'Created'), h('td', { className: 'small muted' }, formatTime(rule.created_at))),
        ),
    );
    const content = h('div', {},
        h('div', { className: 'muted small' }, 'Content:'),
        h('pre', {
            className: 'config-content',
            // UX-review fix: dark-theme legible — mantle bg + text color + surface1 border
            style: 'white-space: pre-wrap; margin: 4px 0; padding: 8px; background: var(--mantle); color: var(--text); border: 1px solid var(--surface1); border-radius: 4px; font-family: monospace; font-size: 12px;',
        }, rule.content || ''),
    );
    const outcomesPanel = outcomes.length === 0
        ? h('p', { className: 'muted small' }, `No outcomes in the last ${data.outcomes_lookback_hours || 168}h.`)
        : h('table', { className: 'config-table', style: 'margin-top: 4px;' },
            h('thead', {}, h('tr', {}, h('th', {}, 'Outcome'), h('th', {}, 'Count'))),
            h('tbody', {}, ...outcomes.map(b => h('tr', {},
                h('td', {}, b.outcome_type),
                h('td', { className: 'config-key' }, String(b.count)),
            ))),
        );
    const outcomesSection = h('div', { style: 'margin-top: 8px;' },
        h('div', { className: 'muted small' }, `Recent outcomes (last ${data.outcomes_lookback_hours || 168}h):`),
        outcomesPanel,
    );
    const editBtn = rule.is_archived
        ? h('span', { className: 'muted small' }, '(archived — un-tombstone via direct Cypher to edit)')
        : h('button', { className: 'btn', onclick: (e) => { e.stopPropagation(); expandedMode = 'edit'; load(); } }, 'Edit');
    const tombBtn = rule.is_archived
        ? h('span', { className: 'muted small' }, '(already archived)')
        : h('button', { className: 'btn btn-warn', onclick: (e) => { e.stopPropagation(); tombstoneRule(rule.constraint_code, rule.content); } }, 'Tombstone');
    const closeBtn = h('button', { className: 'btn', onclick: (e) => { e.stopPropagation(); expandedNodeID = null; load(); } }, 'Close');
    return h('div', {}, meta, content, outcomesSection,
        h('div', { className: 'action-row', style: 'margin-top: 10px; gap: 8px;' }, editBtn, tombBtn, closeBtn),
    );
}

function renderEditForm(rule, spaceID) {
    // Prefill from existing rule; disable code (identity is stable across edits)
    const roleSel = mkSelect(['constraint', 'correction'], rule.role_type);
    const sevSel = mkSelect(['must', 'must_not', 'should', 'note'], rule.constraint_type);
    const catSel = mkSelect([['false', 'actionable'], ['true', 'informational']], rule.is_informational ? 'true' : 'false');
    const scopeSel = mkSelect(['', ...SCOPE_FAMILIES], rule.scope || '');
    const codeInput = h('input', {
        type: 'text', className: 'input', value: rule.constraint_code || '', disabled: true,
        // UX-review fix: use theme-defined muted surface, not light-theme #eee
        style: 'background: var(--surface1); color: var(--overlay1); border: 1px solid var(--surface2); padding: 6px;',
    });
    const contentInput = h('textarea', {
        className: 'input', rows: 5,
        style: 'width: 100%; font-family: inherit; background: var(--mantle); color: var(--text); border: 1px solid var(--surface1); padding: 6px;',
    });
    contentInput.value = rule.content || '';
    const errorLine = h('div', { className: 'error', style: 'display: none; margin-top: 8px;' });
    const state = { override_dedup: false };

    const save = async () => {
        errorLine.style.display = 'none';
        const newContent = contentInput.value.trim();
        if (!newContent) {
            errorLine.textContent = 'Content is required.';
            errorLine.style.display = '';
            return;
        }
        const body = {
            space_id: spaceID,
            role_type: roleSel.value,
            constraint_type: sevSel.value,
            is_informational: catSel.value === 'true',
            content: newContent,
            scope: scopeSel.value,
            constraint_code: rule.constraint_code, // reuse for supersede
        };
        try {
            // Step 1: tombstone-supersede the current live version
            await api.rulesTombstone(rule.constraint_code, spaceID,
                `ui_edit_supersede_${new Date().toISOString().replace(/[:.]/g, '-')}`);
            // Step 2: create the new version with same code
            const resp = await api.rulesCreate(body, true /* override_dedup — expected identical content class */);
            const d = resp.data || {};
            alert(`Rule updated (supersede): new node_id=${d.node_id}. Old version archived (reversible via un-tombstone).`);
            expandedMode = 'view';
            load();
        } catch (err) {
            // On 503, the shipped API body has the flag-flip instructions
            // verbatim — surface it as-is. On other errors, show err.message
            // + suggest the check.
            const isFlagOff = err.status === 503;
            errorLine.textContent = isFlagOff
                ? `Save disabled: ${err.message}`
                : `Save failed: ${err.message}`;
            errorLine.style.display = '';
        }
    };
    const cancel = () => { expandedMode = 'view'; load(); };
    return h('div', {},
        h('div', { className: 'muted small', style: 'margin-bottom: 4px;' },
            `Editing "${rule.constraint_code}" — Save uses tombstone-and-recreate (round-1 immutable lock). Old version stays archived; reversible via un-tombstone.`),
        h('table', { className: 'config-table' },
            h('tbody', {},
                h('tr', {}, h('td', {}, 'Type'), h('td', {}, roleSel)),
                h('tr', {}, h('td', {}, 'Severity'), h('td', {}, sevSel)),
                h('tr', {}, h('td', {}, 'Category'), h('td', {}, catSel)),
                h('tr', {}, h('td', {}, 'Scope'), h('td', {}, scopeSel)),
                h('tr', {}, h('td', {}, 'Code (immutable)'), h('td', {}, codeInput)),
                h('tr', {}, h('td', { style: 'vertical-align: top;' }, 'Content'), h('td', {}, contentInput)),
            ),
        ),
        errorLine,
        h('div', { className: 'action-row', style: 'margin-top: 10px; gap: 8px;' },
            h('button', { className: 'btn btn-primary', onclick: (e) => { e.stopPropagation(); save(); } }, 'Save'),
            h('button', { className: 'btn', onclick: (e) => { e.stopPropagation(); cancel(); } }, 'Cancel'),
        ),
    );
}

// helper: mkSelect(options, selected) — accepts ['a','b','c'] or [['val','label'],...]
function mkSelect(options, selected) {
    const sel = h('select', { className: 'select' });
    for (const o of options) {
        const value = Array.isArray(o) ? o[0] : o;
        const label = Array.isArray(o) ? o[1] : (o || '— none —');
        const opt = h('option', { value }, label);
        if (value === selected) opt.selected = true;
        sel.appendChild(opt);
    }
    return sel;
}

async function tombstoneRule(code, contentHead) {
    const preview = (contentHead || '').slice(0, 100);
    const reason = prompt(
        `Tombstone rule "${code}"?\n\n` +
        `Content: ${preview}${contentHead && contentHead.length > 100 ? '…' : ''}\n\n` +
        `Reversible via direct Cypher (deliberate — un-tombstone stays operator-gated).\n\n` +
        `Reason (optional; blank = ui_tombstone_<timestamp>):`,
    );
    if (reason === null) return;
    const spaceID = state.get('selectedSpace') || 'mdemg-dev';
    try {
        const resp = await api.rulesTombstone(code, spaceID, (reason || '').trim());
        const d = resp.data || {};
        alert(`Tombstoned: ${d.constraint_code} (previous_state=${d.previous_state}).`);
        expandedNodeID = null;
        currentFilters.include_archived = true; // let operator confirm
        load();
    } catch (err) {
        const prefix = err.status === 503 ? 'Tombstone disabled' : 'Tombstone failed';
        alert(`${prefix}: ${err.message}`);
    }
}

// ─── Add-rule form (unchanged from prior Epic 4) ───────────────────────────

function renderAddForm(spaceID) {
    const state = { override_dedup: false };
    const roleSel = mkSelect(['constraint', 'correction'], 'constraint');
    const sevSel = mkSelect(['must', 'must_not', 'should', 'note'], 'must');
    const catSel = mkSelect([['false', 'actionable'], ['true', 'informational']], 'false');
    const scopeSel = mkSelect(['', ...SCOPE_FAMILIES], '');
    const codeInput = h('input', { type: 'text', className: 'input', placeholder: 'optional mnemonic (auto-minted if blank)' });
    const contentInput = h('textarea', {
        className: 'input', rows: 4,
        placeholder: 'Rule text — write it the way you want Jiminy to surface it (e.g., "NEVER commit directly to main branch.")',
        style: 'width: 100%; font-family: inherit; background: var(--mantle); color: var(--text); border: 1px solid var(--surface1); padding: 6px;',
    });
    const dedupWarn = h('div', { style: 'display: none; margin-top: 8px;' });
    const errorLine = h('div', { className: 'error', style: 'display: none; margin-top: 8px;' });
    const submit = async () => {
        errorLine.style.display = 'none';
        dedupWarn.style.display = 'none';
        const body = {
            space_id: spaceID,
            role_type: roleSel.value,
            constraint_type: sevSel.value,
            is_informational: catSel.value === 'true',
            content: contentInput.value.trim(),
            scope: scopeSel.value,
            constraint_code: codeInput.value.trim(),
        };
        if (!body.content) {
            errorLine.textContent = 'Content is required.';
            errorLine.style.display = '';
            return;
        }
        try {
            const resp = await api.rulesCreate(body, state.override_dedup);
            const d = resp.data || {};
            alert(`Rule created: ${d.constraint_code} (node_id=${d.node_id})`);
            showAddForm = false;
            state.override_dedup = false;
            load();
        } catch (err) {
            if (err.status === 409 && err.payload && err.payload.data && err.payload.data.similar_rules) {
                const similar = err.payload.data.similar_rules;
                dedupWarn.innerHTML = '';
                dedupWarn.appendChild(h('div', { className: 'muted small' },
                    `⚠️ ${similar.length} similar rule${similar.length === 1 ? '' : 's'} already exist (sim ≥ threshold):`));
                for (const s of similar) {
                    dedupWarn.appendChild(h('div', { className: 'small', style: 'margin-left: 12px;' },
                        h('code', {}, s.constraint_code),
                        ` — ${s.role_type} — sim=${s.similarity.toFixed(3)} — `,
                        h('span', { className: 'muted' }, (s.content_head || '').slice(0, 100)),
                    ));
                }
                dedupWarn.appendChild(h('div', { className: 'action-row', style: 'margin-top: 8px;' },
                    h('button', { className: 'btn btn-warn', onclick: async () => { state.override_dedup = true; await submit(); } }, 'Create anyway (override dedup)'),
                    h('button', { className: 'btn', onclick: () => { dedupWarn.style.display = 'none'; } }, 'Cancel'),
                ));
                dedupWarn.style.display = '';
                return;
            }
            errorLine.textContent = err.message;
            errorLine.style.display = '';
        }
    };
    return h('div', {
        // UX-review fix: use surface1 (theme-defined) instead of #ccc fallback
        style: 'border: 1px solid var(--surface1); background: var(--surface0); color: var(--text); padding: 12px; margin-top: 12px; margin-bottom: 12px; border-radius: 4px;',
    },
        sectionHeader('Add Rule'),
        h('table', { className: 'config-table' },
            h('tbody', {},
                h('tr', {}, h('td', {}, 'Type'), h('td', {}, roleSel)),
                h('tr', {}, h('td', {}, 'Severity'), h('td', {}, sevSel)),
                h('tr', {}, h('td', {}, 'Category'), h('td', {}, catSel)),
                h('tr', {}, h('td', {}, 'Scope'), h('td', {}, scopeSel)),
                h('tr', {}, h('td', {}, 'Code'), h('td', {}, codeInput)),
                h('tr', {}, h('td', { style: 'vertical-align: top;' }, 'Content'), h('td', {}, contentInput)),
            ),
        ),
        errorLine, dedupWarn,
        h('div', { className: 'action-row', style: 'margin-top: 12px; gap: 8px;' },
            h('button', { className: 'btn btn-primary', onclick: submit }, 'Publish'),
            h('button', { className: 'btn', onclick: () => { showAddForm = false; load(); } }, 'Cancel'),
        ),
    );
}

// ─── Help panel ────────────────────────────────────────────────────────────

function renderHelpPanel() {
    return helpPanel('How the Rules tab works', [
        { term: 'Search box', description: 'Client-side filter across node_id (unique CUIDv2 per rule), constraint_code (mnemonic), and content. Paste a full node_id to jump to that exact rule; type a substring to narrow the list.' },
        { term: 'Click a row', description: 'Expands the rule inline (accordion). Click again — or the Close button — to collapse.' },
        { term: 'Publish = live', description: 'New rules become part of Jiminy\'s active surfacing pool immediately. Actionable rules are subject to strict-mode block; informational rules surface without blocking.' },
        { term: 'Edit = supersede (round-1 immutable lock)', description: 'Save triggers 2 API calls: tombstone the current version with archive_reason=ui_edit_supersede + create a new version with the same constraint_code. Full audit trail preserved; reversible via un-tombstone of the archived version.' },
        { term: 'Dedup warn ≥ 0.75', description: 'On Add, cosine similarity is computed against every live constraint/correction. If any ≥ JIMINY_RULES_DEDUP_SIM_THRESHOLD, you\'ll see a warning + Create-anyway override.' },
        { term: 'Tombstone = soft delete', description: 'is_archived=true + archive_reason. Rule stops surfacing (per JIMINY-ARCHIVED-CODE-FILTER-001). Reversible via direct Cypher — operator-gated.' },
        { term: 'Arc-safety flag', description: 'JIMINY_RULES_UI_WRITE_ENABLED gates Add + Edit + Tombstone. Default false through 2026-08-19; READ endpoints unaffected.' },
    ]);
}

function formatTime(ts) {
    if (!ts) return '—';
    try { return new Date(ts).toLocaleString(); } catch { return ts; }
}
