// rsic.js — RSIC tab: status, lifecycle, trigger action + Grafana link
import * as state from '../state.js';
import * as api from '../api.js';
import { h, infoRow, sectionHeader, statusBadge, btn, clear, helpPanel } from '../utils/dom.js';

let container;

export function render(el) {
    container = el;
    buildUI();
    state.subscribe('rsicHealth', () => updateStatus());
}

export async function poll() {
    try {
        const health = await api.selfImproveHealth();
        state.set('rsicHealth', health);
    } catch {
        // Silent fail
    }
}

function updateStatus() {
    const health = state.get('rsicHealth');
    const statusEl = container?.querySelector('#rsic-status-badge');
    const stateEl = container?.querySelector('#rsic-state-badge');
    if (!statusEl || !stateEl) return;

    if (health) {
        const overall = health.watchdog?.decay_score != null
            ? (health.watchdog.decay_score < 0.3 ? 'healthy' : health.watchdog.decay_score < 0.7 ? 'degraded' : 'critical')
            : 'ok';
        statusEl.replaceChildren(statusBadge(overall));
        const running = health.watchdog?.decay_score != null;
        stateEl.replaceChildren(statusBadge(running ? 'running' : 'stopped'));
    } else {
        statusEl.replaceChildren(statusBadge('unknown'));
        stateEl.replaceChildren(statusBadge('unknown'));
    }
}

function buildUI() {
    const spaceId = () => state.get('selectedSpace') || 'mdemg-dev';

    // RSIC Service section
    const statusBadgeEl = h('span', { id: 'rsic-status-badge' }, statusBadge('unknown'));
    const stateBadgeEl = h('span', { id: 'rsic-state-badge' }, statusBadge('unknown'));

    const statusRow = infoRow('Status', '');
    statusRow.querySelector('.info-value').replaceChildren(statusBadgeEl);
    const stateRow = infoRow('State', '');
    stateRow.querySelector('.info-value').replaceChildren(stateBadgeEl);

    const startBtn = btn('Start', async () => {
        startBtn.disabled = true;
        try {
            const result = await api.rsicStart();
            alert(`RSIC: ${result.status}`);
        } catch (err) { alert(`Start failed: ${err.message}`); }
        finally { startBtn.disabled = false; }
    });
    const stopBtn = btn('Stop', async () => {
        stopBtn.disabled = true;
        try {
            const result = await api.rsicStop();
            alert(`RSIC: ${result.status}`);
        } catch (err) { alert(`Stop failed: ${err.message}`); }
        finally { stopBtn.disabled = false; }
    });
    const restartBtn = btn('Restart', async () => {
        restartBtn.disabled = true;
        try {
            const result = await api.rsicRestart();
            alert(`RSIC: ${result.status}`);
        } catch (err) { alert(`Restart failed: ${err.message}`); }
        finally { restartBtn.disabled = false; }
    });

    // Trigger section
    const tierSelect = h('select', { className: 'select' },
        h('option', { value: 'micro' }, 'Micro'),
        h('option', { value: 'meso', selected: true }, 'Meso'),
        h('option', { value: 'macro' }, 'Macro'),
    );

    const dryRunCheck = h('input', { type: 'checkbox', id: 'rsic-dryrun' });
    const dryRunLabel = h('label', { for: 'rsic-dryrun', className: 'checkbox-label' }, ' Dry run');

    const resultDiv = h('div', { className: 'rsic-result' });

    const triggerBtn = btn('Trigger Cycle', async () => {
        triggerBtn.disabled = true;
        triggerBtn.textContent = 'Running...';
        resultDiv.textContent = '';
        try {
            const result = await api.triggerCycle(spaceId(), tierSelect.value, dryRunCheck.checked);
            const lines = [];
            if (result.cycle_id) lines.push(`Cycle: ${result.cycle_id}`);
            if (result.tier) lines.push(`Tier: ${result.tier}`);
            if (result.outcome) lines.push(`Outcome: ${result.outcome}`);
            if (result.duration_ms) lines.push(`Duration: ${result.duration_ms}ms`);
            if (result.improvements) lines.push(`Improvements: ${result.improvements}`);
            if (result.dry_run) lines.push('(dry run)');
            resultDiv.textContent = lines.join(' | ') || JSON.stringify(result);
            resultDiv.className = 'rsic-result success';
        } catch (err) {
            resultDiv.textContent = `Error: ${err.message}`;
            resultDiv.className = 'rsic-result error';
        } finally {
            triggerBtn.disabled = false;
            triggerBtn.textContent = 'Trigger Cycle';
        }
    }, 'btn-primary');

    const grafanaPort = state.get('grafanaPort') || '3000';

    clear(container,
        sectionHeader('RSIC Service'),
        h('div', { className: 'info-group' }, statusRow, stateRow),
        h('div', { className: 'action-row' }, startBtn, stopBtn, restartBtn),
        sectionHeader('Trigger RSIC Cycle'),
        h('div', { className: 'action-row' },
            tierSelect,
            h('span', { className: 'checkbox-group' }, dryRunCheck, dryRunLabel),
            triggerBtn,
        ),
        resultDiv,
        sectionHeader('Detailed Metrics'),
        h('p', { className: 'muted' },
            'For health scores, watchdog, cycle history, calibration, and safety metrics:'),
        h('a', {
            href: `http://localhost:${grafanaPort}/d/mdemg-rsic`,
            target: 'grafana',
            className: 'grafana-link large',
        }, '\u2197 Open RSIC Dashboard in Grafana'),
        helpPanel('Help', [
            { term: 'Status', description: 'RSIC health status derived from watchdog decay score: healthy (< 0.3), degraded (0.3-0.7), critical (> 0.7). Polled from GET /v1/self-improve/health.' },
            { term: 'State', description: 'RSIC watchdog runtime state: running (actively monitoring) or stopped (watchdog inactive).' },
            { term: 'Start/Stop/Restart', description: 'Control the RSIC watchdog lifecycle. Start resumes monitoring, Stop pauses it, Restart stops then starts with a fresh context.' },
            { term: 'RSIC', description: 'Recursive Self-Improvement Cycle. RSIC periodically assesses memory quality, identifies improvements, and applies them. Each cycle: assess \u2192 reflect \u2192 plan \u2192 execute.' },
            { term: 'Tier', description: 'Cycle scope. Micro: fast, single-dimension fixes (e.g., prune stale edges). Meso: moderate, multi-dimension improvements (default). Macro: comprehensive, full-graph analysis and restructuring.' },
            { term: 'Dry Run', description: 'When checked, the cycle runs assessment and planning but does not execute any changes. Use to preview what RSIC would do.' },
            { term: 'Trigger Cycle', description: 'Manually starts an RSIC cycle on the selected space. Shows cycle ID, tier, outcome, duration, and number of improvements when complete.' },
            { term: 'Grafana RSIC Dashboard', description: 'Links to the full RSIC Grafana dashboard with 8 health dimensions, cycle history, watchdog escalation, safety blocks, calibration progress, and persistence metrics.' },
        ]),
    );
}
