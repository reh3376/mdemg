// rules.js — JIMINY-RULES-UI-001 Epic 4: Rules tab
//
// Three views (all in one tab, driven by internal state):
//   1. List view (default) — filterable table + row-click opens detail
//   2. Detail side-panel — rule + recent outcomes + [Tombstone] button
//   3. Add-rule form — modal-style overlay + dedup-warn inline
//
// WRITE surface (Add + Tombstone) is flag-gated server-side by
// JiminyRulesUIWriteEnabled; the tab surfaces a clean disabled state
// when the flag is off (which is the default during the
// JIMINY-CEILING-BREAK-2 arc window through 2026-08-19).

import * as api from '../api.js';
import * as state from '../state.js';
import { h, sectionHeader, clear, helpPanel } from '../utils/dom.js';

// LEVER-C-TIGHTEN-002's 9 scope families — the "scope" dropdown reuses
// them so a rule's scope also feeds the surfacing filter (one source of
// truth). See internal/jiminy/scope_gate.go.
const SCOPE_FAMILIES = [
    'git', 'file_mutation', 'bash', 'schema', 'identifier',
    'testing', 'process_docs', 'llm_config', 'cms',
];

let container;
let currentFilters = {};
let currentDetailCode = null; // when set, detail side-panel is open
let showAddForm = false;

export function render(el) {
    container = el;
    load();
}

async function load() {
    const spaceID = state.get('selectedSpace') || 'mdemg-dev';
    try {
        const listResp = await api.rulesList(spaceID, { ...currentFilters, limit: 200 });
        const items = (listResp.data && listResp.data.items) || [];
        // If detail is open, refresh it in parallel.
        let detail = null;
        if (currentDetailCode) {
            try {
                const dResp = await api.rulesDetail(currentDetailCode, spaceID);
                detail = dResp.data;
            } catch (err) {
                detail = { error: err.message };
            }
        }
        renderTab(items, detail, spaceID);
    } catch (err) {
        clear(container, h('p', { className: 'error' }, `Failed to load rules: ${err.message}`));
    }
}

function renderTab(items, detail, spaceID) {
    const panels = [
        renderToolbar(spaceID, items.length),
    ];
    if (showAddForm) {
        panels.push(renderAddForm(spaceID));
    }
    panels.push(renderListPanel(items));
    if (detail) {
        panels.push(renderDetailPanel(detail));
    }
    panels.push(renderHelpPanel());
    clear(container, ...panels);
}

// ─── Toolbar (filters + Add button) ────────────────────────────────────────

function renderToolbar(spaceID, count) {
    const filterEls = [];

    const mkSelect = (key, label, opts) => {
        const sel = h('select', {
            className: 'select',
            onchange: (e) => {
                if (e.target.value) currentFilters[key] = e.target.value;
                else delete currentFilters[key];
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

    filterEls.push(mkSelect('type', 'Type', [
        { value: 'constraint', label: 'constraint' },
        { value: 'correction', label: 'correction' },
    ]));
    filterEls.push(mkSelect('severity', 'Severity', [
        { value: 'must', label: 'must' },
        { value: 'must_not', label: 'must_not' },
        { value: 'should', label: 'should' },
        { value: 'note', label: 'note' },
    ]));
    filterEls.push(mkSelect('category', 'Category', [
        { value: 'actionable', label: 'actionable' },
        { value: 'informational', label: 'informational' },
    ]));
    filterEls.push(mkSelect('scope', 'Scope', SCOPE_FAMILIES.map(s => ({ value: s, label: s }))));

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

    return h('div', {},
        sectionHeader(`Jiminy Rules — space=${spaceID}`),
        h('p', { className: 'muted small' },
            `${count} rule${count === 1 ? '' : 's'} shown. WRITE endpoints (Add + Tombstone) are `,
            h('b', {}, 'flag-gated'),
            ' by JIMINY_RULES_UI_WRITE_ENABLED (default false through 2026-08-19; flip in .env + restart to enable).',
        ),
        h('div', { className: 'action-row', style: 'gap: 8px; flex-wrap: wrap;' },
            ...filterEls,
            archivedCheck,
            addBtn,
        ),
    );
}

// ─── List panel ────────────────────────────────────────────────────────────

function renderListPanel(items) {
    if (items.length === 0) {
        return h('div', {},
            h('p', { className: 'muted' }, 'No rules match the current filters. Clear filters or ',
                h('code', {}, 'include archived'), ' to see more.'),
        );
    }
    const headerRow = h('tr', {},
        h('th', {}, 'Code'),
        h('th', {}, 'Type'),
        h('th', {}, 'Severity'),
        h('th', {}, 'Category'),
        h('th', {}, 'Scope'),
        h('th', {}, 'Content (head)'),
        h('th', {}, 'Archived'),
    );
    const rows = items.map(it => {
        const isDetailOpen = it.constraint_code === currentDetailCode;
        return h('tr', {
            className: isDetailOpen ? 'row-selected' : '',
            style: 'cursor: pointer;',
            onclick: () => openDetail(it.constraint_code),
        },
            h('td', { className: 'config-key' }, it.constraint_code || '—'),
            h('td', {}, it.role_type),
            h('td', {}, it.constraint_type || '—'),
            h('td', {}, it.is_informational ? h('span', { className: 'badge badge-warn' }, 'informational') : 'actionable'),
            h('td', { className: 'small' }, it.scope || '—'),
            h('td', { className: 'small' }, (it.content || '').slice(0, 120) + (it.content && it.content.length > 120 ? '…' : '')),
            h('td', { className: 'small muted' }, it.is_archived ? h('span', { className: 'badge badge-default' }, 'archived') : ''),
        );
    });
    return h('div', {},
        h('table', { className: 'config-table' }, h('thead', {}, headerRow), h('tbody', {}, ...rows)),
    );
}

function openDetail(code) {
    currentDetailCode = code;
    load();
}

function closeDetail() {
    currentDetailCode = null;
    load();
}

// ─── Detail side-panel ────────────────────────────────────────────────────

function renderDetailPanel(data) {
    if (data.error) {
        return h('div', { className: 'panel-detail' },
            sectionHeader('Detail'),
            h('p', { className: 'error' }, `Failed: ${data.error}`),
            h('button', { className: 'btn', onclick: closeDetail }, 'Close'),
        );
    }
    const rule = data.rule || {};
    const outcomes = data.recent_outcomes || [];

    const metaRows = h('table', { className: 'config-table' },
        h('tbody', {},
            h('tr', {}, h('td', {}, 'Code'), h('td', { className: 'config-key' }, rule.constraint_code || '—')),
            h('tr', {}, h('td', {}, 'Node ID'), h('td', { className: 'small' }, rule.node_id || '—')),
            h('tr', {}, h('td', {}, 'Type'), h('td', {}, rule.role_type || '—')),
            h('tr', {}, h('td', {}, 'Severity'), h('td', {}, rule.constraint_type || '—')),
            h('tr', {}, h('td', {}, 'Category'), h('td', {}, rule.is_informational ? 'informational' : 'actionable')),
            h('tr', {}, h('td', {}, 'Scope'), h('td', {}, rule.scope || '—')),
            h('tr', {}, h('td', {}, 'Archived'), h('td', {}, rule.is_archived ? `yes — ${rule.archive_reason || ''}` : 'no')),
            h('tr', {}, h('td', {}, 'Created'), h('td', { className: 'small muted' }, formatTime(rule.created_at))),
        ),
    );

    const outcomesPanel = outcomes.length === 0
        ? h('p', { className: 'muted small' }, `No outcomes in the last ${data.outcomes_lookback_hours || 168}h — either this rule doesn't surface, or it does but the auto-classifier hasn't classified against it recently.`)
        : h('table', { className: 'config-table' },
            h('thead', {}, h('tr', {}, h('th', {}, 'Outcome'), h('th', {}, 'Count'))),
            h('tbody', {}, ...outcomes.map(b => h('tr', {},
                h('td', {}, b.outcome_type),
                h('td', { className: 'config-key' }, String(b.count)),
            ))),
        );

    const tombstoneBtn = rule.is_archived
        ? h('span', { className: 'muted small' }, '(already archived — re-tombstone refreshes archive_reason)')
        : h('button', {
            className: 'btn btn-warn',
            onclick: () => tombstoneRule(rule.constraint_code, rule.content),
        }, 'Tombstone');

    return h('div', { className: 'panel-detail', style: 'border: 1px solid var(--border, #ccc); padding: 12px; margin-top: 12px;' },
        sectionHeader('Rule Detail'),
        metaRows,
        h('div', { style: 'margin-top: 10px;' },
            h('div', { className: 'muted small' }, 'Content:'),
            h('pre', { className: 'config-content', style: 'white-space: pre-wrap; margin: 4px 0;' }, rule.content || ''),
        ),
        h('div', { style: 'margin-top: 10px;' },
            h('div', { className: 'muted small' }, `Recent outcomes (last ${data.outcomes_lookback_hours || 168}h):`),
            outcomesPanel,
        ),
        h('div', { className: 'action-row', style: 'margin-top: 12px; gap: 8px;' },
            tombstoneBtn,
            h('button', { className: 'btn', onclick: closeDetail }, 'Close'),
        ),
    );
}

async function tombstoneRule(code, contentHead) {
    const preview = (contentHead || '').slice(0, 100);
    const reason = prompt(
        `Tombstone rule "${code}"?\n\n` +
        `Content: ${preview}${contentHead && contentHead.length > 100 ? '…' : ''}\n\n` +
        `Reversible via direct Cypher (deliberate — un-tombstone stays operator-gated).\n\n` +
        `Reason (optional; blank = ui_tombstone_<timestamp>):`,
    );
    if (reason === null) return; // cancelled
    const spaceID = state.get('selectedSpace') || 'mdemg-dev';
    try {
        const resp = await api.rulesTombstone(code, spaceID, reason.trim());
        const d = resp.data || {};
        alert(`Tombstoned: ${d.constraint_code} (previous_state=${d.previous_state}). Archive reason: ${d.archive_reason}`);
        currentDetailCode = null;
        // Force refresh with archived visible so operator can confirm.
        currentFilters.include_archived = true;
        load();
    } catch (err) {
        // 503 flag-off is the expected shape during arc window.
        alert(`Tombstone failed: ${err.message}`);
    }
}

// ─── Add-rule form ─────────────────────────────────────────────────────────

function renderAddForm(spaceID) {
    const state = { override_dedup: false, similar: [] };

    const roleSel = h('select', { className: 'select' },
        h('option', { value: 'constraint', selected: true }, 'constraint'),
        h('option', { value: 'correction' }, 'correction'),
    );
    const sevSel = h('select', { className: 'select' },
        h('option', { value: 'must', selected: true }, 'must'),
        h('option', { value: 'must_not' }, 'must_not'),
        h('option', { value: 'should' }, 'should'),
        h('option', { value: 'note' }, 'note'),
    );
    const catSel = h('select', { className: 'select' },
        h('option', { value: 'false', selected: true }, 'actionable'),
        h('option', { value: 'true' }, 'informational'),
    );
    const scopeSel = h('select', { className: 'select' },
        h('option', { value: '' }, '— none —'),
        ...SCOPE_FAMILIES.map(s => h('option', { value: s }, s)),
    );
    const codeInput = h('input', {
        type: 'text', className: 'input',
        placeholder: 'optional mnemonic (auto-minted if blank)',
    });
    const contentInput = h('textarea', {
        className: 'input', rows: 4,
        placeholder: 'Rule text — write it the way you want Jiminy to surface it to the LLM (e.g., "NEVER commit directly to main branch — all work happens on dev branches.")',
        style: 'width: 100%; font-family: inherit;',
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
                // Dedup warn — render inline, offer override.
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
                    h('button', {
                        className: 'btn btn-warn',
                        onclick: async () => { state.override_dedup = true; await submit(); },
                    }, 'Create anyway (override dedup)'),
                    h('button', { className: 'btn', onclick: () => { dedupWarn.style.display = 'none'; } }, 'Cancel'),
                ));
                dedupWarn.style.display = '';
                return;
            }
            errorLine.textContent = err.message;
            errorLine.style.display = '';
        }
    };

    return h('div', { style: 'border: 1px solid var(--border, #ccc); padding: 12px; margin-top: 12px; margin-bottom: 12px;' },
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
        errorLine,
        dedupWarn,
        h('div', { className: 'action-row', style: 'margin-top: 12px; gap: 8px;' },
            h('button', { className: 'btn btn-primary', onclick: submit }, 'Publish'),
            h('button', { className: 'btn', onclick: () => { showAddForm = false; load(); } }, 'Cancel'),
        ),
    );
}

// ─── Help panel ─────────────────────────────────────────────────────────

function renderHelpPanel() {
    return helpPanel('How the Rules tab works', [
        { term: 'Publish = live', description: 'New rules become part of Jiminy\'s active surfacing pool immediately (per JIMINY-RULES-UI-001 round-2 design). Actionable rules are subject to strict-mode block on Write/Edit/Bash; informational rules surface without blocking.' },
        { term: 'Dedup warn ≥ 0.75', description: 'On save, cosine similarity is computed against every live constraint/correction. If any ≥ JIMINY_RULES_DEDUP_SIM_THRESHOLD (default 0.75, CREATE-CORRECTION-DEDUP-001 threshold), you\'ll see a warning + list. Confirm-anyway bypasses.' },
        { term: 'Tombstone = soft delete', description: 'Sets is_archived=true + archive_reason. The rule stops surfacing in Jiminy (per JIMINY-ARCHIVED-CODE-FILTER-001 reader-side filter). Reversible via direct Cypher — operator-gated by design.' },
        { term: 'Immutable + tombstone-and-recreate', description: 'No in-place edit. To "edit" a rule: tombstone the old + publish a new one. Matches the JIMINY-CORPUS-001 tombstone-safety pattern; no schema migration required.' },
        { term: 'Arc-safety flag', description: 'JIMINY_RULES_UI_WRITE_ENABLED gates Add + Tombstone. Default false through the JIMINY-CEILING-BREAK-2 arc window (2026-08-19); READ endpoints unaffected.' },
    ]);
}

function formatTime(ts) {
    if (!ts) return '—';
    try { return new Date(ts).toLocaleString(); } catch { return ts; }
}
