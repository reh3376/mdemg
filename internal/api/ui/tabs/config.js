// config.js — Config tab: effective configuration table
import * as api from '../api.js';
import { h, sectionHeader, clear, helpPanel } from '../utils/dom.js';

let container;
let allConfig = [];

export function render(el) {
    container = el;
    clear(container, h('p', { className: 'muted' }, 'Loading configuration...'));
    load();
}

async function load() {
    try {
        const data = await api.adminConfig();
        allConfig = data.config || [];
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
        { term: 'Value', description: 'Current effective value. Sensitive values (API keys, passwords) are masked as ****.' },
        { term: 'Source', description: 'Where the value comes from. env (green) = environment variable, yaml (blue) = config.yaml file, default (gray) = built-in default. Env vars take precedence over YAML, which takes precedence over defaults.' },
        { term: 'Filter', description: 'Type in the search box to filter config entries by key or value. Matching is case-insensitive and updates instantly.' },
        { term: 'YAML Path', description: 'The filesystem path to the active config.yaml file, shown below the header. If no YAML file is found, all values come from env vars or defaults.' },
    ]));

    clear(container, ...sections);
}

function renderRows(table, entries) {
    const tbody = table.querySelector('tbody');
    const rows = entries.map(e => {
        const sourceCls = e.source === 'env' ? 'badge-env' : e.source === 'yaml' ? 'badge-yaml' : 'badge-default';
        return h('tr', {},
            h('td', { className: 'config-key' }, e.key),
            h('td', { className: 'config-value' }, e.masked ? '****' : e.value),
            h('td', {}, h('span', { className: `source-badge ${sourceCls}` }, e.source)),
        );
    });
    tbody.replaceChildren(...rows);
}
