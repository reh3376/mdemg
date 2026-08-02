// jiminy.js — Jiminy mode selector (JIMINY-MODE-001, 2026-08-02)
//
// Operator-facing UI for the strict/suggest mode toggle.
//   strict  = enforce (block Write/Edit + alert on WARNED+ violations)
//   suggest = advisory only (guidance surfaces, no blocking, no alerts)
//
// Reads current state from GET /v1/jiminy/strict; flips via POST.
// The state file at ~/.mdemg/.jiminy-strict-mode persists across restarts
// (JIMINY-ENFORCE-001), so operator preference survives a server reboot.

import * as api from '../api.js';
import { h, sectionHeader, btn, clear, helpPanel } from '../utils/dom.js';

let container;
let currentSessionID = 'claude-core';

export function render(el) {
    container = el;
    clear(container, h('p', { className: 'muted' }, 'Loading Jiminy mode...'));
    load();
}

async function load() {
    try {
        // Try to resolve the operator's default session key from the state file
        // via the server's boot config.
        const resp = await api.jiminyStrictGet(currentSessionID);
        const data = (resp && resp.data) || {};
        renderPanel(data);
    } catch (err) {
        clear(container, h('p', { className: 'error' }, `Failed to load Jiminy mode: ${err.message}`));
    }
}

function renderPanel(data) {
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

    const actionRow = h('div', { className: 'action-row' }, strictBtn, suggestBtn);

    clear(container,
        sectionHeader('Jiminy Enforcement Mode'),
        h('p', { className: 'muted' },
            'Jiminy governs the main LLM\'s behavior. In ',
            h('b', {}, 'strict'),
            ' mode, Write/Edit tool calls that violate WARNED+ escalated constraints are ',
            h('b', {}, 'blocked'),
            ' and a HIGH-severity alert is emitted. In ',
            h('b', {}, 'suggest'),
            ' mode, guidance is still surfaced to the agent context but no blocking or alerting occurs.',
        ),
        rows,
        actionRow,
        helpPanel('How the modes differ', [
            { term: 'Strict', description: 'Enforces hard constraints via the pre-write-check hook. Fail-open if the server is unreachable (a persistent warning stays until reconnect). Recommended for production sessions.' },
            { term: 'Suggest', description: 'Advisory only. The agent still receives Jiminy guidance in every prompt but nothing blocks its tool calls. Recommended for exploratory work where speed matters more than enforcement.' },
            { term: 'Session-scoped', description: 'This toggle affects only the named session. Change JIMINY_MODE in .env for the process-wide boot default (requires restart).' },
            { term: 'Persistence', description: 'The state file at ~/.mdemg/.jiminy-strict-mode persists across restarts (JIMINY-ENFORCE-001), so operator preference survives a server reboot.' },
        ]),
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
