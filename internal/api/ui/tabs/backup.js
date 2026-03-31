// backup.js — Backup & Restore tab
import * as api from '../api.js';
import { h, sectionHeader, btn, clear, infoRow, helpPanel } from '../utils/dom.js';

let container;
let statusTimer = null;

const STATUS_BADGES = {
    completed: 'badge-ok',
    pending: 'badge-warn',
    running: 'badge-warn',
    in_progress: 'badge-warn',
    failed: 'badge-err',
    cancelled: 'badge-default',
};

const TYPE_BADGES = {
    full: 'badge-ok',
    partial_space: 'badge-yaml',
};

export function render(el) {
    container = el;
    stopPolling();
    clear(container, h('p', { className: 'muted' }, 'Loading backups...'));
    load();
}

function stopPolling() {
    if (statusTimer) {
        clearInterval(statusTimer);
        statusTimer = null;
    }
}

async function load() {
    try {
        const data = await api.backupList();
        const backups = data.backups || [];
        renderBackups(backups);
    } catch (err) {
        clear(container,
            h('p', { className: 'error' }, `Failed to load backups: ${err.message}`),
            helpSection(),
        );
    }
}

function renderBackups(backups) {
    const sections = [];

    // --- Trigger Backup ---
    sections.push(sectionHeader('Trigger Backup'));
    sections.push(triggerForm());

    // --- Backup List ---
    sections.push(sectionHeader('Backup History'));
    sections.push(filterRow());

    if (backups.length === 0) {
        sections.push(h('p', { className: 'muted' }, 'No backups found. Trigger a backup to get started.'));
    } else {
        sections.push(backupTable(backups));
    }

    // --- Restore Section ---
    sections.push(sectionHeader('Restore'));
    sections.push(restoreForm(backups));

    // --- Active Operations ---
    sections.push(h('div', { id: 'backup-active-ops' }));

    // --- Help ---
    sections.push(helpSection());

    clear(container, ...sections);
}

function triggerForm() {
    const spaceInput = h('input', {
        type: 'text',
        className: 'config-input',
        placeholder: 'Space ID (e.g. mdemg-dev)',
        value: 'mdemg-dev',
        style: { width: '200px' },
    });

    const typeSelect = h('select', { className: 'config-input', style: { width: '160px' } },
        h('option', { value: 'full' }, 'Full'),
        h('option', { value: 'partial_space', selected: true }, 'Partial (Space)'),
    );

    const goBtn = btn('Trigger Backup', async () => {
        const space = spaceInput.value.trim();
        if (!space) { alert('Space ID is required'); return; }
        try {
            goBtn.disabled = true;
            goBtn.textContent = 'Triggering...';
            const result = await api.triggerBackup(space, typeSelect.value);
            startStatusPolling(result.backup_id, 'backup');
            await load();
        } catch (err) {
            alert(`Trigger failed: ${err.message}`);
        } finally {
            goBtn.disabled = false;
            goBtn.textContent = 'Trigger Backup';
        }
    });

    return h('div', { className: 'action-row' }, spaceInput, typeSelect, goBtn);
}

function filterRow() {
    const select = h('select', {
        className: 'config-input',
        style: { width: '160px' },
        id: 'backup-type-filter',
        onchange: async () => {
            try {
                const data = await api.backupList(select.value);
                const tbl = container.querySelector('#backup-list-table');
                if (tbl) tbl.replaceWith(backupTable(data.backups || []));
            } catch (err) { /* ignore filter errors */ }
        },
    },
        h('option', { value: '' }, 'All Types'),
        h('option', { value: 'full' }, 'Full'),
        h('option', { value: 'partial_space' }, 'Partial'),
    );

    return h('div', { className: 'action-row' },
        h('span', { className: 'muted' }, 'Filter by type: '),
        select,
        btn('Refresh', () => load()),
    );
}

function backupTable(backups) {
    const thead = h('tr', {},
        h('th', {}, 'ID'),
        h('th', {}, 'Type'),
        h('th', {}, 'Space'),
        h('th', {}, 'Created'),
        h('th', {}, 'Size'),
        h('th', {}, 'Status'),
        h('th', {}, 'Actions'),
    );

    const rows = backups.map(b => {
        const id = b.backup_id || b.id || '—';
        const shortId = id.length > 12 ? id.slice(0, 12) + '…' : id;
        const type = b.type || '—';
        const space = b.space_id || b.space_ids?.join(', ') || '—';
        const created = b.created ? new Date(b.created).toLocaleString() : '—';
        const size = b.size ? formatSize(b.size) : '—';
        const status = b.status || 'unknown';
        const typeCls = TYPE_BADGES[type] || 'badge-default';
        const statusCls = STATUS_BADGES[status] || 'badge-default';

        return h('tr', {},
            h('td', { title: id }, shortId),
            h('td', {}, h('span', { className: `source-badge ${typeCls}` }, type)),
            h('td', {}, space),
            h('td', {}, created),
            h('td', {}, size),
            h('td', {}, h('span', { className: `badge ${statusCls}` }, status)),
            h('td', {},
                h('div', { className: 'action-row' },
                    btn('Manifest', () => showManifest(id)),
                    btn('Delete', () => deleteBackup(id), 'btn-danger'),
                ),
            ),
        );
    });

    return h('table', { className: 'info-table', id: 'backup-list-table' },
        h('thead', {}, thead),
        h('tbody', {}, ...rows),
    );
}

function restoreForm(backups) {
    const completed = backups.filter(b => (b.status || '') === 'completed');

    if (completed.length === 0) {
        return h('p', { className: 'muted' }, 'No completed backups available for restore.');
    }

    const select = h('select', { className: 'config-input', style: { width: '300px' } },
        ...completed.map(b => {
            const id = b.backup_id || b.id;
            const label = `${id.slice(0, 12)}… (${b.type || '?'}, ${b.created ? new Date(b.created).toLocaleDateString() : '?'})`;
            return h('option', { value: id }, label);
        }),
    );

    const restoreBtn = btn('Restore', async () => {
        const backupId = select.value;
        if (!backupId) return;
        if (!window.confirm(`Restore from backup ${backupId.slice(0, 12)}…? This will overwrite current data.`)) return;
        try {
            restoreBtn.disabled = true;
            restoreBtn.textContent = 'Restoring...';
            const result = await api.backupRestore(backupId);
            startStatusPolling(result.restore_id, 'restore');
        } catch (err) {
            alert(`Restore failed: ${err.message}`);
        } finally {
            restoreBtn.disabled = false;
            restoreBtn.textContent = 'Restore';
        }
    }, 'btn-danger');

    return h('div', { className: 'action-row' }, select, restoreBtn);
}

function startStatusPolling(id, kind) {
    stopPolling();
    const opsDiv = container.querySelector('#backup-active-ops');
    if (!opsDiv) return;

    const statusEl = h('div', { className: 'info-row' },
        h('span', { className: 'info-label' }, `Active ${kind}:`),
        h('span', { className: 'info-value badge badge-warn' }, 'polling...'),
    );
    opsDiv.replaceChildren(sectionHeader('Active Operation'), statusEl);

    statusTimer = setInterval(async () => {
        try {
            const data = kind === 'backup'
                ? await api.backupStatus(id)
                : await api.restoreStatus(id);
            const status = data.status || 'unknown';
            const statusCls = STATUS_BADGES[status] || 'badge-default';
            const progress = data.progress ? ` (${data.progress}%)` : '';
            statusEl.querySelector('.info-value').className = `info-value badge ${statusCls}`;
            statusEl.querySelector('.info-value').textContent = `${status}${progress}`;

            if (status === 'completed' || status === 'failed' || status === 'cancelled') {
                stopPolling();
                if (status === 'completed') await load();
            }
        } catch {
            stopPolling();
            opsDiv.replaceChildren();
        }
    }, 5000);
}

async function showManifest(id) {
    try {
        const m = await api.backupManifest(id);
        alert(JSON.stringify(m, null, 2));
    } catch (err) {
        alert(`Failed to load manifest: ${err.message}`);
    }
}

async function deleteBackup(id) {
    if (!window.confirm(`Delete backup ${id.slice(0, 12)}…? This cannot be undone.`)) return;
    try {
        await api.backupDelete(id);
        await load();
    } catch (err) {
        alert(`Delete failed: ${err.message}`);
    }
}

function formatSize(bytes) {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function helpSection() {
    return helpPanel('Backup Help', [
        { term: 'Full Backup', description: 'Backs up all spaces, nodes, edges, and metadata. Use for complete disaster recovery.' },
        { term: 'Partial Backup', description: 'Backs up a single space. Faster and smaller than full backups.' },
        { term: 'Restore', description: 'Restores data from a completed backup. Overwrites current data for the affected spaces.' },
        { term: 'Manifest', description: 'Shows backup metadata: type, checksum, size, and creation timestamp.' },
        { term: 'Status Polling', description: 'While a backup or restore is in progress, status is polled every 5 seconds automatically.' },
    ]);
}
