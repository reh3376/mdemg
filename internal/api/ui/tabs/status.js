// status.js — Status tab: health badges + Grafana dashboard links
import * as state from '../state.js';
import * as api from '../api.js';
import { h, infoRow, sectionHeader, statusBadge, btn, clear, helpPanel } from '../utils/dom.js';

let container;

export function render(el) {
    container = el;
    clear(container, h('p', { className: 'muted' }, 'Loading status...'));
    state.subscribe('healthz', () => update());
    state.subscribe('readyz', () => update());
}

function update() {
    const hz = state.get('healthz');
    const rz = state.get('readyz');
    if (!container) return;

    const sections = [];

    // Server health
    sections.push(sectionHeader('Server'));
    if (hz) {
        const statusRow = infoRow('Status', '');
        statusRow.querySelector('.info-value').replaceChildren(statusBadge(hz.status || 'unknown'));
        const stateRow = infoRow('State', '');
        stateRow.querySelector('.info-value').replaceChildren(statusBadge('running'));
        sections.push(h('div', { className: 'info-group' },
            statusRow,
            stateRow,
            infoRow('Version', hz.version || '\u2014'),
        ));
    } else {
        const statusRow = infoRow('Status', '');
        statusRow.querySelector('.info-value').replaceChildren(statusBadge('unreachable'));
        const stateRow = infoRow('State', '');
        stateRow.querySelector('.info-value').replaceChildren(statusBadge('unreachable'));
        sections.push(h('div', { className: 'info-group' },
            statusRow,
            stateRow,
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

    // Server actions
    sections.push(sectionHeader('Server Actions'));
    const restartBtn = btn('Restart Server', async () => {
        if (!confirm('Restart the MDEMG server? The dashboard will be temporarily unavailable.')) return;
        restartBtn.disabled = true;
        restartBtn.textContent = 'Restarting...';
        try {
            await api.serverRestart();
            // Poll healthz until server returns
            const poll = setInterval(async () => {
                try {
                    await api.healthz();
                    clearInterval(poll);
                    location.reload();
                } catch { /* still restarting */ }
            }, 2000);
        } catch (err) {
            alert(`Restart failed: ${err.message}`);
            restartBtn.disabled = false;
            restartBtn.textContent = 'Restart Server';
        }
    }, 'btn-danger');
    sections.push(h('div', { className: 'action-row' }, restartBtn));

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
            h('a', { href: `${base}${path}`, target: 'grafana', className: 'grafana-link' },
                `\u2197 ${name}`),
        ));
    }
    sections.push(linkList);

    sections.push(helpPanel('Help', [
        { term: 'Status', description: 'Server health reported by GET /healthz: ok (all checks pass), degraded (some checks failing), or unreachable (server not responding). Polled every 10 seconds.' },
        { term: 'State', description: 'Server runtime state: running (responding to requests), unreachable (not responding). Derived from healthz availability.' },
        { term: 'Version', description: 'MDEMG server version reported by the /healthz endpoint.' },
        { term: 'Services', description: 'Readiness checks from GET /readyz. Each service (Neo4j, embeddings, plugins) reports its own status. All must be "ready" for the server to accept traffic.' },
        { term: 'Restart Server', description: 'Gracefully restarts the MDEMG server binary. The dashboard will be temporarily unavailable during restart. The page auto-reloads when the server is back.' },
        { term: 'Grafana Dashboards', description: 'Links to 7 Grafana dashboards for detailed time-series metrics. Clicking a link navigates the existing Grafana tab (or opens one if none exists). Dashboards cover: request metrics, Neo4j stats, RSIC cycles, graph topology, Jiminy guidance, J17 protocol, and fine-tuning training data.' },
    ]));

    clear(container, ...sections);
}
