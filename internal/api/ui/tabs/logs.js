// logs.js — Logs tab: searchable, color-coded log viewer
import * as api from '../api.js';
import { h, sectionHeader, clear } from '../utils/dom.js';

let container;
let allEntries = [];
let levelFilter = '';
let searchFilter = '';

export function render(el) {
    container = el;
    buildUI();
    load();
}

export async function poll() {
    await load();
}

function buildUI() {
    const levelSelect = h('select', { className: 'select' },
        h('option', { value: '' }, 'All Levels'),
        h('option', { value: 'DEBUG' }, 'DEBUG'),
        h('option', { value: 'INFO' }, 'INFO'),
        h('option', { value: 'WARN' }, 'WARN'),
        h('option', { value: 'ERROR' }, 'ERROR'),
    );
    levelSelect.onchange = () => { levelFilter = levelSelect.value; renderEntries(); };

    const searchInput = h('input', {
        type: 'text',
        placeholder: 'Search logs...',
        className: 'search-input',
    });
    searchInput.oninput = () => { searchFilter = searchInput.value.toLowerCase(); renderEntries(); };

    const logContainer = h('div', { id: 'log-entries', className: 'log-container' });

    clear(container,
        sectionHeader('Server Logs'),
        h('div', { className: 'log-controls' }, levelSelect, searchInput),
        logContainer,
    );
}

async function load() {
    try {
        const data = await api.adminLogs(500);
        allEntries = data.entries || [];
        renderEntries();
    } catch {
        // Silent fail on polling
    }
}

function renderEntries() {
    const logEl = container?.querySelector('#log-entries');
    if (!logEl) return;

    let filtered = allEntries;
    if (levelFilter) {
        filtered = filtered.filter(e => e.level === levelFilter);
    }
    if (searchFilter) {
        filtered = filtered.filter(e =>
            e.message.toLowerCase().includes(searchFilter) ||
            e.raw.toLowerCase().includes(searchFilter));
    }

    const rows = filtered.map(e => {
        const lvl = (e.level || 'INFO').toUpperCase();
        const cls = lvl === 'ERROR' ? 'log-error' : lvl === 'WARN' ? 'log-warn' : lvl === 'DEBUG' ? 'log-debug' : 'log-info';
        const ts = e.timestamp ? new Date(e.timestamp).toLocaleTimeString() : '';
        return h('div', { className: `log-line ${cls}` },
            h('span', { className: 'log-ts' }, ts),
            h('span', { className: `log-level ${cls}` }, lvl),
            h('span', { className: 'log-msg' }, e.message),
        );
    });

    logEl.replaceChildren(...rows);
}
