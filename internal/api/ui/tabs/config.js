// config.js — Config tab: editable configuration table
import * as api from '../api.js';
import { h, sectionHeader, btn, clear, helpPanel } from '../utils/dom.js';

let container;
let allConfig = [];
let dirtyFields = new Map(); // key → new value

// Known dropdown options for specific config keys.
const DROPDOWNS = {
    'embedding.provider': ['', 'ollama', 'openai'],
    'llm.provider': ['ollama', 'openai'],
    'ingest.speed': ['', 'fast', 'balanced', 'thorough'],
    'jiminy.synthesis_provider': ['', 'ollama', 'openai'],
    'jiminy.evaluate_llm_provider': ['', 'ollama', 'openai'],
};

// Boolean config keys (rendered as checkboxes).
const BOOLEANS = new Set([
    'plugins.enabled', 'backup.enabled',
    'jiminy.enabled', 'jiminy.synthesis_enabled',
    'jiminy.evaluate_enabled', 'jiminy.evaluate_llm_enabled',
    'jiminy.outcome_llm_enabled', 'jiminy.j17_enabled',
    'retrieval.graph_typed_edges_enabled',
]);

export function render(el) {
    container = el;
    clear(container, h('p', { className: 'muted' }, 'Loading configuration...'));
    load();
}

async function load() {
    try {
        const data = await api.adminConfig();
        allConfig = data.config || [];
        dirtyFields.clear();
        renderTable(allConfig, data.yaml_path);
    } catch (err) {
        clear(container, h('p', { className: 'error' }, `Failed to load config: ${err.message}`));
    }
}

function renderTable(entries, yamlPath) {
    const sections = [];

    sections.push(sectionHeader('Effective Configuration'));
    if (yamlPath) {
        sections.push(h('p', { className: 'muted small' }, `YAML: ${yamlPath}`));
    }

    // Save bar (hidden until dirty)
    const saveBar = h('div', { className: 'config-save-bar hidden', id: 'config-save-bar' },
        btn('Save All', async () => {
            if (dirtyFields.size === 0) return;
            saveBar.querySelector('.btn').disabled = true;
            try {
                await api.updateConfig(Object.fromEntries(dirtyFields));
                dirtyFields.clear();
                await load();
            } catch (err) {
                alert(`Save failed: ${err.message}`);
            }
        }, 'btn-primary'),
        h('span', { className: 'muted small', id: 'dirty-count' }, ''),
    );
    sections.push(saveBar);

    // Search filter
    const searchInput = h('input', {
        type: 'text',
        placeholder: 'Filter keys...',
        className: 'search-input',
    });
    searchInput.oninput = () => {
        const q = searchInput.value.toLowerCase();
        const filtered = allConfig.filter(e => e.key.toLowerCase().includes(q) || e.value.toLowerCase().includes(q));
        renderRows(table, filtered);
    };
    sections.push(searchInput);

    // Table
    const table = h('table', { className: 'config-table' },
        h('thead', {},
            h('tr', {},
                h('th', {}, 'Key'),
                h('th', {}, 'Value'),
                h('th', {}, 'Source'),
            ),
        ),
        h('tbody', {}),
    );
    renderRows(table, entries);
    sections.push(table);

    sections.push(helpPanel('Help', [
        { term: 'Key', description: 'Configuration key in dot-notation (e.g., neo4j.uri). Maps to a YAML path or environment variable.' },
        { term: 'Value', description: 'Current effective value. Editable fields show an input, dropdown, or checkbox. Changed values turn red until saved.' },
        { term: 'Source', description: 'Where the value comes from. env (green) = environment variable (read-only), yaml (blue) = config.yaml file (editable), default (gray) = built-in default (editable via YAML).' },
        { term: 'Read-Only Fields', description: 'Fields sourced from environment variables or containing masked secrets (****) cannot be edited in the UI. Change them via .env file or environment variables.' },
        { term: 'Save All', description: 'Appears when any field has been changed. Writes all pending changes to the YAML config file. Changes take effect after server restart.' },
        { term: 'Filter', description: 'Type in the search box to filter config entries by key or value. Matching is case-insensitive and updates instantly.' },
        { term: 'YAML Path', description: 'The filesystem path to the active config.yaml file, shown below the header. If no YAML file is found, fields are read-only.' },
    ]));

    clear(container, ...sections);
}

function isReadOnly(entry) {
    return entry.source === 'env' || entry.masked;
}

function createInput(entry) {
    const key = entry.key;
    const val = entry.masked ? '****' : entry.value;

    if (isReadOnly(entry)) {
        const span = h('span', { className: 'config-value config-readonly' }, val);
        if (entry.source === 'env') {
            span.append(h('span', { className: 'config-lock', title: 'Set via environment variable' }, ' \uD83D\uDD12'));
        }
        return span;
    }

    // Boolean → checkbox
    if (BOOLEANS.has(key)) {
        const cb = h('input', {
            type: 'checkbox',
            checked: val === 'true',
            className: 'config-checkbox',
        });
        // DASHBOARD-FIXES-001: single handler (the first assignment was dead —
        // immediately overwritten by this one, which also updates the label).
        const label = h('label', { className: 'config-cb-label' }, cb, val === 'true' ? ' true' : ' false');
        cb.onchange = () => {
            const newVal = cb.checked ? 'true' : 'false';
            label.lastChild.textContent = cb.checked ? ' true' : ' false';
            markDirty(key, newVal, val, label.closest('tr'));
        };
        return label;
    }

    // Dropdown
    if (DROPDOWNS[key]) {
        const select = h('select', { className: 'config-input' });
        for (const opt of DROPDOWNS[key]) {
            const option = h('option', { value: opt, selected: opt === val }, opt || '(empty)');
            select.append(option);
        }
        select.onchange = () => {
            markDirty(key, select.value, val, select.closest('tr'));
        };
        return select;
    }

    // Text input
    const input = h('input', {
        type: 'text',
        value: val,
        className: 'config-input',
    });
    input.onchange = () => {
        markDirty(key, input.value, val, input.closest('tr'));
    };
    return input;
}

function markDirty(key, newVal, originalVal, row) {
    if (newVal === originalVal) {
        dirtyFields.delete(key);
        row?.classList.remove('config-dirty-row');
    } else {
        dirtyFields.set(key, newVal);
        row?.classList.add('config-dirty-row');
    }
    updateSaveBar();
}

function updateSaveBar() {
    const bar = container?.querySelector('#config-save-bar');
    const count = container?.querySelector('#dirty-count');
    if (!bar) return;
    if (dirtyFields.size > 0) {
        bar.classList.remove('hidden');
        if (count) count.textContent = `${dirtyFields.size} unsaved change${dirtyFields.size > 1 ? 's' : ''}`;
    } else {
        bar.classList.add('hidden');
    }
}

function renderRows(table, entries) {
    const tbody = table.querySelector('tbody');
    const rows = entries.map(e => {
        const sourceCls = e.source === 'env' ? 'badge-env' : e.source === 'yaml' ? 'badge-yaml' : 'badge-default';
        const isDirty = dirtyFields.has(e.key);
        const tr = h('tr', { className: isDirty ? 'config-dirty-row' : '' },
            h('td', { className: 'config-key' }, e.key),
            h('td', {}, createInput(e)),
            h('td', {}, h('span', { className: `source-badge ${sourceCls}` }, e.source)),
        );
        return tr;
    });
    tbody.replaceChildren(...rows);
}
