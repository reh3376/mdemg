// training_data.js — Training Data Export tab
import * as api from '../api.js';
import { h, sectionHeader, btn, clear, infoRow, helpPanel } from '../utils/dom.js';

let container;
let statusTimer = null;

const STATUS_BADGES = {
    completed: 'badge-ok',
    pending: 'badge-warn',
    running: 'badge-warn',
    failed: 'badge-err',
    cancelled: 'badge-default',
};

export function render(el) {
    container = el;
    stopPolling();
    clear(container,
        sectionHeader('Training Data Export'),
        exportForm(),
        h('div', { id: 'export-status-area' }),
        sectionHeader('Export Guide'),
        helpSection(),
    );
}

function stopPolling() {
    if (statusTimer) {
        clearInterval(statusTimer);
        statusTimer = null;
    }
}

function exportForm() {
    const form = h('div', { className: 'card' });

    const spaceInput = h('input', {
        type: 'text',
        className: 'input',
        id: 'export-space-id',
        placeholder: 'Space ID (e.g., mdemg-dev)',
        value: 'mdemg-dev',
    });

    const sinceInput = h('input', {
        type: 'text',
        className: 'input',
        id: 'export-since',
        placeholder: 'From (RFC3339, optional)',
    });

    const untilInput = h('input', {
        type: 'text',
        className: 'input',
        id: 'export-until',
        placeholder: 'To (RFC3339, optional)',
    });

    const llmCheck = h('input', { type: 'checkbox', id: 'tbl-llm', checked: true });
    const retCheck = h('input', { type: 'checkbox', id: 'tbl-retrieval', checked: true });
    const embCheck = h('input', { type: 'checkbox', id: 'tbl-embedding' });

    const tableChecks = h('div', { className: 'flex gap-sm' },
        h('label', {}, llmCheck, ' LLM Interactions'),
        h('label', {}, retCheck, ' Retrieval Events'),
        h('label', {}, embCheck, ' Embedding Events'),
    );

    const exportBtn = btn('Export', async () => {
        exportBtn.disabled = true;
        exportBtn.textContent = 'Exporting...';

        const tables = [];
        if (llmCheck.checked) tables.push('llm_interactions');
        if (retCheck.checked) tables.push('retrieval_events');
        if (embCheck.checked) tables.push('embedding_events');

        if (tables.length === 0) {
            showStatus('error', 'Select at least one table');
            exportBtn.disabled = false;
            exportBtn.textContent = 'Export';
            return;
        }

        try {
            const req = {
                space_id: spaceInput.value || 'mdemg-dev',
                tables,
            };
            if (sinceInput.value) req.from = sinceInput.value;
            if (untilInput.value) req.to = untilInput.value;

            const data = await api.trainingDataExport(req);
            showStatus('ok', `Export started: ${data.export_id}`);
            startStatusPolling(data.export_id);
        } catch (err) {
            showStatus('error', `Export failed: ${err.message}`);
        }

        exportBtn.disabled = false;
        exportBtn.textContent = 'Export';
    });

    form.append(
        infoRow('Space ID', spaceInput),
        infoRow('From', sinceInput),
        infoRow('To', untilInput),
        infoRow('Tables', tableChecks),
        h('div', { className: 'flex gap-sm mt-sm' }, exportBtn),
    );

    return form;
}

function showStatus(type, message) {
    const area = document.getElementById('export-status-area');
    if (!area) return;

    const badge = type === 'ok' ? 'badge-ok' : type === 'error' ? 'badge-err' : 'badge-warn';
    const el = h('div', { className: `card mt-sm` },
        h('span', { className: badge }, type.toUpperCase()),
        ' ',
        message,
    );

    area.prepend(el);
}

function startStatusPolling(exportId) {
    stopPolling();

    const poll = async () => {
        try {
            const data = await api.trainingDataStatus(exportId);
            const area = document.getElementById('export-status-area');
            if (!area) { stopPolling(); return; }

            const badge = STATUS_BADGES[data.status] || 'badge-default';
            const statusCard = h('div', { className: 'card mt-sm', id: `status-${exportId}` });

            const items = [
                infoRow('Export ID', data.export_id),
                infoRow('Status', h('span', { className: badge }, data.status)),
            ];

            if (data.progress && data.progress.phase) {
                items.push(infoRow('Phase', data.progress.phase));
            }
            if (data.progress && data.progress.percentage > 0) {
                items.push(infoRow('Progress', `${data.progress.percentage.toFixed(1)}%`));
            }

            if (data.status === 'completed' && data.result) {
                items.push(infoRow('Total Rows', data.result.total_rows));
                items.push(infoRow('Tables', data.result.tables));
                items.push(infoRow('Privacy', `${data.result.privacy || 0} violations`));

                const dlBtn = btn('Download', () => {
                    window.location = api.trainingDataDownloadURL(exportId);
                });
                items.push(h('div', { className: 'mt-sm' }, dlBtn));
            }

            if (data.error) {
                items.push(infoRow('Error', h('span', { className: 'badge-err' }, data.error)));
            }

            statusCard.append(...items);

            const existing = document.getElementById(`status-${exportId}`);
            if (existing) {
                existing.replaceWith(statusCard);
            } else {
                area.prepend(statusCard);
            }

            if (data.status === 'completed' || data.status === 'failed' || data.status === 'cancelled') {
                stopPolling();
            }
        } catch (err) {
            showStatus('error', `Status poll failed: ${err.message}`);
            stopPolling();
        }
    };

    poll();
    statusTimer = setInterval(poll, 5000);
}

function helpSection() {
    return helpPanel([
        'Training data exports are UTDS-compliant .tar.gz archives.',
        'Each archive contains manifest.json + JSONL data files.',
        'Privacy scanning runs on ALL text fields — export is blocked if PII is detected.',
        'After export, use the curation pipeline:',
        '  1. quality_filter.py — apply quality gates',
        '  2. format_converter.py — convert to MLX chat format',
        '  3. dataset_versioner.py — temporal split + versioning',
        '',
        'CLI: mdemg data export --space-id mdemg-dev',
        'Validate: python utds_runner.py validate --archive export.tar.gz',
    ]);
}
