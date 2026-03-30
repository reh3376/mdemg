// status.js — Status tab: health badges + Grafana dashboard links
import * as state from '../state.js';
import { h, infoRow, sectionHeader, statusBadge, clear } from '../utils/dom.js';

let container;

export function render(el) {
    container = el;
    clear(container, h('p', { className: 'muted' }, 'Loading status...'));
    state.subscribe('healthz', () => update());
    state.subscribe('readyz', () => update());
    state.subscribe('embeddingHealth', () => update());
}

function update() {
    const hz = state.get('healthz');
    const rz = state.get('readyz');
    const eh = state.get('embeddingHealth');
    if (!container) return;

    const sections = [];

    // Server health
    sections.push(sectionHeader('Server'));
    if (hz) {
        sections.push(h('div', { className: 'info-group' },
            infoRow('Status', ''),
            (() => { const r = infoRow('Status', ''); r.querySelector('.info-value').replaceChildren(statusBadge(hz.status || 'unknown')); return r; })(),
            infoRow('Version', hz.version || '\u2014'),
        ));
    } else {
        sections.push(h('div', { className: 'info-group' },
            infoRow('Status', ''),
            (() => { const r = infoRow('Status', ''); r.querySelector('.info-value').replaceChildren(statusBadge('unreachable')); return r; })(),
        ));
    }

    // Readiness checks
    if (rz) {
        sections.push(sectionHeader('Services'));
        const checks = rz.checks || rz;
        const group = h('div', { className: 'info-group' });
        if (typeof checks === 'object') {
            for (const [name, detail] of Object.entries(checks)) {
                if (name === 'status') continue;
                const status = (typeof detail === 'object' && detail.status) ? detail.status : String(detail);
                const row = infoRow(name, '');
                row.querySelector('.info-value').replaceChildren(statusBadge(status));
                group.append(row);
            }
        }
        sections.push(group);
    }

    // Embedding health
    if (eh) {
        sections.push(sectionHeader('Embeddings'));
        sections.push(h('div', { className: 'info-group' },
            infoRow('Provider', eh.provider || '\u2014'),
            infoRow('Model', eh.model || '\u2014'),
            infoRow('Dimensions', eh.dimensions || '\u2014'),
            infoRow('Status', eh.status || '\u2014'),
        ));
    }

    // Grafana dashboard links
    sections.push(sectionHeader('Grafana Dashboards'));
    const grafanaPort = state.get('grafanaPort') || '3000';
    const base = `http://localhost:${grafanaPort}`;
    const dashboards = [
        ['Overview', '/d/mdemg-overview'],
        ['Neo4j', '/d/mdemg-neo4j'],
        ['RSIC Operations', '/d/mdemg-rsic'],
        ['Graph Topology', '/d/mdemg-graph-topology'],
        ['Jiminy Guidance', '/d/mdemg-jiminy'],
        ['J17 Protocol', '/d/mdemg-j17'],
        ['FT Training', '/d/mdemg-ft-training'],
    ];
    const linkList = h('div', { className: 'info-group' });
    for (const [name, path] of dashboards) {
        linkList.append(h('div', { className: 'info-row' },
            h('a', { href: `${base}${path}`, target: '_blank', className: 'grafana-link' },
                `\u2197 ${name}`),
        ));
    }
    sections.push(linkList);

    clear(container, ...sections);
}
