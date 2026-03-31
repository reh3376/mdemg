// plugins.js — Plugin Manager tab
import * as api from '../api.js';
import { h, sectionHeader, btn, clear, helpPanel } from '../utils/dom.js';
import { subscribe, clearSubscribers } from '../state.js';

let container;

const TYPE_BADGES = {
    INGESTION: 'badge-ok',
    REASONING: 'badge-yaml',
    APE: 'badge-default',
    CRUD: 'badge-warn',
};

const STATE_BADGES = {
    ready: 'badge-ok',
    running: 'badge-ok',
    starting: 'badge-warn',
    unhealthy: 'badge-err',
    stopped: 'badge-default',
    crashed: 'badge-err',
    stopping: 'badge-warn',
};

export function render(el) {
    container = el;
    clear(container, h('p', { className: 'muted' }, 'Loading plugins...'));
    load();
}

async function load() {
    try {
        const data = await api.pluginList();
        const plugins = data.data?.plugins || data.plugins || data.data || [];
        renderPlugins(plugins);
    } catch (err) {
        clear(container, h('p', { className: 'error' }, `Failed to load plugins: ${err.message}`));
    }
}

function renderPlugins(plugins) {
    const sections = [];

    sections.push(sectionHeader('Installed Plugins'));

    if (plugins.length === 0) {
        sections.push(h('p', { className: 'muted' }, 'No plugins installed. Place plugins in the plugins directory and restart.'));
    } else {
        for (const p of plugins) {
            sections.push(pluginCard(p));
        }
    }

    sections.push(helpPanel('Help', [
        { term: 'Plugin Types', description: 'INGESTION (green) — data ingestion modules. REASONING (blue) — analysis/reasoning modules. APE (gray) — self-improvement modules. CRUD (yellow) — entity management modules.' },
        { term: 'States', description: 'ready = running and healthy. starting = initializing. unhealthy = health check failing. stopped = intentionally stopped. crashed = exited unexpectedly.' },
        { term: 'Start/Stop/Restart', description: 'Control individual plugin lifecycle. Stop gracefully shuts down the plugin process. Start launches the binary and performs gRPC handshake. Restart does both.' },
        { term: 'Validate', description: 'Runs full validation suite on the plugin: manifest checks, binary checks, gRPC connectivity, capability verification.' },
    ]));

    clear(container, ...sections);
}

function pluginCard(p) {
    const typeCls = TYPE_BADGES[p.type] || 'badge-default';
    const state = p.state || p.status || 'unknown';
    const stateCls = STATE_BADGES[state] || 'badge-default';

    const detailDiv = h('div', { className: 'plugin-detail hidden' });

    const card = h('div', { className: 'plugin-card' },
        h('div', { className: 'plugin-card-header' },
            h('div', { className: 'plugin-card-title' },
                h('span', { className: 'plugin-name' }, p.name || p.id),
                h('span', { className: `source-badge ${typeCls}` }, p.type),
                h('span', { className: `badge ${stateCls}` }, state),
            ),
            h('div', { className: 'plugin-card-meta' },
                p.version ? h('span', { className: 'muted small' }, `v${p.version}`) : null,
            ),
        ),
        h('div', { className: 'action-row' },
            btn('Start', () => lifecycle(p.id, 'start')),
            btn('Stop', () => lifecycle(p.id, 'stop')),
            btn('Restart', () => lifecycle(p.id, 'restart')),
            btn('Validate', () => validate(p.id)),
            btn(detailDiv.classList.contains('hidden') ? 'Details' : 'Hide', () => {
                detailDiv.classList.toggle('hidden');
                loadDetail(p.id, detailDiv);
            }),
        ),
        detailDiv,
    );

    return card;
}

async function lifecycle(id, action) {
    try {
        if (action === 'start') await api.pluginStart(id);
        else if (action === 'stop') await api.pluginStop(id);
        else if (action === 'restart') await api.pluginRestart(id);
        await load();
    } catch (err) {
        alert(`Plugin ${action} failed: ${err.message}`);
    }
}

async function validate(id) {
    try {
        const result = await api.pluginValidate(id);
        const data = result.data || result;
        alert(data.valid ? `Plugin ${id} is valid.` : `Plugin ${id} validation failed.`);
    } catch (err) {
        alert(`Validate failed: ${err.message}`);
    }
}

async function loadDetail(id, el) {
    if (!el.classList.contains('hidden') && el.children.length > 0) return;
    el.textContent = 'Loading...';
    try {
        const result = await api.pluginDetail(id);
        const info = result.data || result;
        const rows = [
            ['ID', info.id],
            ['PID', info.pid || 'n/a'],
            ['Socket', info.socket_path || 'n/a'],
            ['Started At', info.started_at || 'n/a'],
            ['Last Healthy', info.last_healthy || 'n/a'],
            ['Capabilities', (info.capabilities || []).join(', ') || 'none'],
        ];
        el.replaceChildren(
            ...rows.map(([label, value]) =>
                h('div', { className: 'info-row' },
                    h('span', { className: 'info-label' }, label),
                    h('span', { className: 'info-value' }, String(value)),
                ),
            ),
        );
    } catch (err) {
        el.textContent = `Error: ${err.message}`;
    }
}
