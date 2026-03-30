// rsic.js — RSIC tab: trigger action + Grafana link
import * as state from '../state.js';
import * as api from '../api.js';
import { h, sectionHeader, btn, clear } from '../utils/dom.js';

let container;

export function render(el) {
    container = el;
    buildUI();
}

function buildUI() {
    const spaceId = () => state.get('selectedSpace') || 'mdemg-dev';

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
            target: '_blank',
            className: 'grafana-link large',
        }, '\u2197 Open RSIC Dashboard in Grafana'),
    );
}
