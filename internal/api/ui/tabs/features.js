// features.js — Services & Features tab
import * as api from '../api.js';
import { h, sectionHeader, btn, clear, helpPanel } from '../utils/dom.js';

let container;

const STATUS_BADGES = {
    healthy: 'badge-ok',
    stopped: 'badge-warn',
    unavailable: 'badge-default',
};

export function render(el) {
    container = el;
    clear(container, h('p', { className: 'muted' }, 'Loading features...'));
    load();
}

async function load() {
    try {
        const data = await api.featureList();
        renderFeatures(data.features || []);
    } catch (err) {
        clear(container, h('p', { className: 'error' }, `Failed to load features: ${err.message}`));
    }
}

function renderFeatures(features) {
    const sections = [];

    const controllable = features.filter(f => f.controllable);
    const configOnly = features.filter(f => !f.controllable);

    sections.push(sectionHeader('Controllable Services'));
    if (controllable.length === 0) {
        sections.push(h('p', { className: 'muted' }, 'No controllable services.'));
    } else {
        sections.push(featureTable(controllable, true));
    }

    sections.push(sectionHeader('Config-Only Services'));
    if (configOnly.length === 0) {
        sections.push(h('p', { className: 'muted' }, 'No config-only services.'));
    } else {
        sections.push(featureTable(configOnly, false));
    }

    sections.push(helpPanel('Help', [
        { term: 'Controllable', description: 'Services with Start/Stop/Restart buttons. These can be controlled at runtime without restarting the server.' },
        { term: 'Config-Only', description: 'Services that are enabled/disabled via configuration. Changes require a config update and server restart to take effect.' },
        { term: 'Status', description: 'healthy (green) = running normally. stopped (yellow) = intentionally stopped. unavailable (gray) = not initialized or not configured.' },
        { term: 'Config Key', description: 'The configuration key that controls whether this service is enabled. Edit in the Config tab.' },
    ]));

    clear(container, ...sections);
}

function featureTable(features, showActions) {
    const headerRow = h('tr', {},
        h('th', {}, 'Service'),
        h('th', {}, 'Status'),
        h('th', {}, 'State'),
        showActions ? h('th', {}, 'Actions') : h('th', {}, 'Config Key'),
    );

    const rows = features.map(f => {
        const statusCls = STATUS_BADGES[f.status] || 'badge-default';
        const tr = h('tr', {},
            h('td', { className: 'config-key' }, f.name),
            h('td', {}, h('span', { className: `badge ${statusCls}` }, f.status)),
            h('td', {}, f.state),
        );

        if (showActions) {
            tr.append(h('td', {},
                h('div', { className: 'action-row' },
                    btn('Start', () => featureAction(f.name, 'start')),
                    btn('Stop', () => featureAction(f.name, 'stop')),
                    btn('Restart', () => featureAction(f.name, 'restart')),
                ),
            ));
        } else {
            tr.append(h('td', { className: 'muted small' }, f.config_key || '—'));
        }
        return tr;
    });

    return h('table', { className: 'config-table' },
        h('thead', {}, headerRow),
        h('tbody', {}, ...rows),
    );
}

async function featureAction(name, action) {
    try {
        if (action === 'start') await api.featureStart(name);
        else if (action === 'stop') await api.featureStop(name);
        else if (action === 'restart') await api.featureRestart(name);
        await load();
    } catch (err) {
        alert(`Feature ${action} failed: ${err.message}`);
    }
}
