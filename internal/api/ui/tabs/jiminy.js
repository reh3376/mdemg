// jiminy.js — Jiminy mode + operator overrides + enforcement timeline
//
// Sections:
//   1. Enforcement Mode (JIMINY-MODE-001, 2026-08-02) — strict/suggest toggle
//   2. Active Overrides (ENFORCE-UI-OVERRIDES, 2026-08-03) — table + revoke buttons
//   3. Recent Enforcement Timeline (ENFORCE-UI-OVERRIDES) — apply/revoke/expire events
//      from constraint_overrides hypertable (ENFORCE-OVERRIDES-TSDB).

import * as api from '../api.js';
import * as state from '../state.js';
import { h, sectionHeader, clear, helpPanel } from '../utils/dom.js';

let container;
let currentSessionID = 'claude-core';

export function render(el) {
    container = el;
    clear(container, h('p', { className: 'muted' }, 'Loading Jiminy…'));
    load();
}

async function load() {
    try {
        const [modeResp, activeResp, historyResp] = await Promise.allSettled([
            api.jiminyStrictGet(currentSessionID),
            api.jiminyOverrideList(''),  // '' = all sessions
            api.jiminyOverrideHistory(state.get('selectedSpace') || 'mdemg-dev', 168),
        ]);

        const mode = modeResp.status === 'fulfilled' ? (modeResp.value.data || {}) : {};
        const active = activeResp.status === 'fulfilled' ? (activeResp.value.data || {}) : { overrides: [], count: 0 };
        const history = historyResp.status === 'fulfilled' ? (historyResp.value.data || {}) : { events: [], count: 0 };

        renderTab(mode, active, history);
    } catch (err) {
        clear(container, h('p', { className: 'error' }, `Failed to load Jiminy: ${err.message}`));
    }
}

function renderTab(modeData, activeData, historyData) {
    const modePanel = renderModePanel(modeData);
    const activePanel = renderActiveOverridesPanel(activeData);
    const historyPanel = renderHistoryPanel(historyData);
    clear(container, modePanel, activePanel, historyPanel);
}

// ─── Section 1: Enforcement Mode ────────────────────────────────────────────

function renderModePanel(data) {
    const currentMode = data.mode || 'suggest';
    const bootDefault = data.boot_default || 'strict';
    const sessionID = data.session_id || currentSessionID;
    currentSessionID = sessionID;

    const modeBadge = currentMode === 'strict'
        ? h('span', { className: 'badge badge-ok' }, 'STRICT (enforcing)')
        : h('span', { className: 'badge badge-warn' }, 'SUGGEST (advisory)');

    const strictBtn = h('button', {
        className: currentMode === 'strict' ? 'btn btn-primary' : 'btn',
        onclick: () => setMode(true),
        disabled: currentMode === 'strict',
    }, 'Enforce (strict)');
    const suggestBtn = h('button', {
        className: currentMode === 'suggest' ? 'btn btn-primary' : 'btn',
        onclick: () => setMode(false),
        disabled: currentMode === 'suggest',
    }, 'Advise (suggest)');

    const rows = h('table', { className: 'config-table' },
        h('tbody', {},
            h('tr', {}, h('td', {}, 'Session'), h('td', { className: 'config-key' }, sessionID)),
            h('tr', {}, h('td', {}, 'Current mode'), h('td', {}, modeBadge)),
            h('tr', {}, h('td', {}, 'Boot default (JIMINY_MODE)'), h('td', { className: 'config-key' }, bootDefault)),
        ),
    );

    return h('div', {},
        sectionHeader('Jiminy Enforcement Mode'),
        h('p', { className: 'muted' },
            'Jiminy governs the main LLM\'s behavior. In ',
            h('b', {}, 'strict'),
            ' mode, Write/Edit + Bash tool calls that violate WARNED+ escalated constraints are ',
            h('b', {}, 'blocked'),
            ' and a HIGH-severity alert is emitted. In ',
            h('b', {}, 'suggest'),
            ' mode, guidance is still surfaced but no blocking or alerting occurs.',
        ),
        rows,
        h('div', { className: 'action-row' }, strictBtn, suggestBtn),
    );
}

async function setMode(enabled) {
    try {
        await api.jiminyStrictSet(currentSessionID, enabled);
        await load();
    } catch (err) {
        alert(`Failed to set Jiminy mode: ${err.message}`);
    }
}

// ─── Section 2: Active Overrides ───────────────────────────────────────────

function renderActiveOverridesPanel(data) {
    const overrides = Array.isArray(data.overrides) ? data.overrides : [];
    const header = h('div', {},
        sectionHeader(`Active Overrides (${overrides.length})`),
        h('p', { className: 'muted' },
            'Time-boxed suppressions installed via ',
            h('code', {}, 'mdemg jiminy override apply'),
            '. Each override lets specific constraint codes pass classifier deny until scheduled expiry. ',
            'Every apply/revoke/expire is JSONL-audited (~/.mdemg/jiminy-overrides.jsonl) and TSDB-persisted (constraint_overrides hypertable).',
        ),
    );

    if (overrides.length === 0) {
        return h('div', {}, header, h('p', { className: 'muted' }, 'No active overrides. Use ', h('code', {}, 'mdemg jiminy override apply --constraint <code> --reason <text> --duration 15m'), ' to install one.'));
    }

    const headerRow = h('tr', {},
        h('th', {}, 'Constraint'),
        h('th', {}, 'Session'),
        h('th', {}, 'Reason'),
        h('th', {}, 'Expires'),
        h('th', {}, 'Actions'),
    );
    const rows = overrides.map(o => h('tr', {},
        h('td', { className: 'config-key' }, o.constraint_code || '—'),
        h('td', {}, o.session_id || '—'),
        h('td', {}, o.reason || '—'),
        h('td', { className: 'small muted' }, formatTime(o.expires_at)),
        h('td', {},
            h('button', {
                className: 'btn btn-warn',
                onclick: () => revokeOverride(o.session_id, o.constraint_code),
            }, 'Revoke'),
        ),
    ));

    return h('div', {}, header, h('table', { className: 'config-table' }, h('thead', {}, headerRow), h('tbody', {}, ...rows)));
}

async function revokeOverride(sessionID, constraintCode) {
    if (!confirm(`Revoke override on "${constraintCode}" for session "${sessionID}"?`)) return;
    try {
        await api.jiminyOverrideRevoke(sessionID, constraintCode);
        await load();
    } catch (err) {
        alert(`Revoke failed: ${err.message}`);
    }
}

// ─── Section 3: Recent Enforcement Timeline ────────────────────────────────

function renderHistoryPanel(data) {
    const events = Array.isArray(data.events) ? data.events : [];
    const header = h('div', {},
        sectionHeader(`Recent Override Timeline (${events.length}, last ${data.hours || 168}h)`),
        h('p', { className: 'muted' },
            'Every apply/revoke/expire op from the ',
            h('code', {}, 'constraint_overrides'),
            ' hypertable. Newest first. Reveals which constraints operators repeatedly override — the RSIC ',
            h('code', {}, 'enforcement_false_positive_high'),
            ' pattern will fire when a code accumulates enough events, recommending deprecate/reword.',
        ),
    );

    if (events.length === 0) {
        return h('div', {}, header, h('p', { className: 'muted' }, 'No override events in the window. This is expected when enforcement is either always-correct or never-triggered — check the Active Overrides + Mode panels above.'));
    }

    const headerRow = h('tr', {},
        h('th', {}, 'Time'),
        h('th', {}, 'Op'),
        h('th', {}, 'Constraint'),
        h('th', {}, 'Session'),
        h('th', {}, 'Reason'),
    );
    const rows = events.slice(0, 50).map(e => h('tr', {},
        h('td', { className: 'small muted' }, formatTime(e.time)),
        h('td', {}, h('span', { className: opBadgeClass(e.op) }, e.op)),
        h('td', { className: 'config-key' }, e.constraint_code || '—'),
        h('td', { className: 'small muted' }, e.session_id || '—'),
        h('td', {}, e.reason || '—'),
    ));

    const footer = events.length > 50 ? h('p', { className: 'muted small' }, `Showing 50 of ${events.length}. Extend the window via the URL query ?hours=336 for 14d.`) : null;

    return h('div', {},
        header,
        h('table', { className: 'config-table' }, h('thead', {}, headerRow), h('tbody', {}, ...rows)),
        footer,
        helpPanel('How overrides fit the enforcement arc', [
            { term: 'Apply → deny suppressed', description: 'When Jiminy would deny a Write/Edit or Bash call, the operator can install a time-boxed override on the specific constraint code. The classifier suppresses the deny and records blocked_false_positive to constraint_outcomes — RSIC learns the code is being overridden.' },
            { term: 'Revoke → early clear', description: 'Removes an active override before its scheduled expiry. Useful when the operator realizes the override is no longer needed.' },
            { term: 'Expire → time-boxed safety', description: 'Every override MUST have a positive duration — forgotten unbounded overrides silently disable enforcement forever. The expire op is logged on the next Get/List call after expiry.' },
        ]),
    );
}

function opBadgeClass(op) {
    switch (op) {
        case 'apply': return 'badge badge-warn';
        case 'revoke': return 'badge badge-ok';
        case 'expire': return 'badge badge-default';
        default: return 'badge badge-default';
    }
}

function formatTime(ts) {
    if (!ts) return '—';
    try {
        const d = new Date(ts);
        return d.toLocaleString();
    } catch {
        return ts;
    }
}
