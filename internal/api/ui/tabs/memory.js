// memory.js — Memory tab: stats + knowledge sharing (export/import)
import * as state from '../state.js';
import * as api from '../api.js';
import { h, infoRow, sectionHeader, btn, clear } from '../utils/dom.js';
import { formatNumber } from '../utils/formatting.js';

let container;

export function render(el) {
    container = el;
    clear(container, h('p', { className: 'muted' }, 'Loading memory stats...'));
    state.subscribe('memoryStats', () => update());
    state.subscribe('memoryDistribution', () => update());
}

function update() {
    const ms = state.get('memoryStats');
    if (!container || !ms) return;
    const dist = state.get('memoryDistribution');

    const sections = [];

    // Overview
    sections.push(sectionHeader('Memory Overview'));
    sections.push(h('div', { className: 'info-group' },
        infoRow('Total Memories', formatNumber(ms.total_nodes)),
        infoRow('Embedding Coverage', ms.embedding_coverage != null ? `${(ms.embedding_coverage * 100).toFixed(1)}%` : '\u2014'),
        infoRow('Health Score', ms.health_score != null ? `${(ms.health_score * 100).toFixed(1)}%` : '\u2014'),
    ));

    // Layer breakdown
    if (ms.by_layer) {
        sections.push(sectionHeader('By Layer'));
        const maxCount = Math.max(...Object.values(ms.by_layer), 1);
        const layerGroup = h('div', { className: 'info-group' });
        for (const [layer, count] of Object.entries(ms.by_layer)) {
            const pct = (count / maxCount) * 100;
            layerGroup.append(h('div', { className: 'bar-row' },
                h('span', { className: 'bar-label' }, layer),
                h('div', { className: 'bar-track' },
                    h('div', { className: 'bar-fill', style: { width: `${pct}%` } })),
                h('span', { className: 'bar-value' }, formatNumber(count)),
            ));
        }
        sections.push(layerGroup);
    }

    // Temporal distribution
    if (ms.temporal) {
        sections.push(sectionHeader('Temporal Distribution'));
        sections.push(h('div', { className: 'info-group' },
            infoRow('Last 24h', formatNumber(ms.temporal.last_24h)),
            infoRow('Last 7d', formatNumber(ms.temporal.last_7d)),
            infoRow('Last 30d', formatNumber(ms.temporal.last_30d)),
            infoRow('Older', formatNumber(ms.temporal.older)),
        ));
    }

    // Connectivity
    if (ms.connectivity) {
        sections.push(sectionHeader('Connectivity'));
        sections.push(h('div', { className: 'info-group' },
            infoRow('Avg Edges', ms.connectivity.avg_edges != null ? ms.connectivity.avg_edges.toFixed(1) : '\u2014'),
            infoRow('Max Edges', formatNumber(ms.connectivity.max_edges)),
            infoRow('Orphan Nodes', formatNumber(ms.connectivity.orphans)),
        ));
    }

    // Knowledge sharing
    sections.push(sectionHeader('Knowledge Sharing'));
    const spaceId = state.get('selectedSpace') || 'mdemg-dev';

    // Export
    const profileSelect = h('select', { className: 'select' },
        h('option', { value: 'full' }, 'Full'),
        h('option', { value: 'shareable' }, 'Shareable'),
        h('option', { value: 'metadata' }, 'Metadata'),
        h('option', { value: 'learned' }, 'Learned'),
        h('option', { value: 'cms' }, 'CMS'),
    );
    const exportBtn = btn('Export', async () => {
        exportBtn.disabled = true;
        exportBtn.textContent = 'Exporting...';
        try {
            const data = await api.spaceExport(spaceId, profileSelect.value);
            const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const a = h('a', { href: url, download: `${spaceId}-${profileSelect.value}.mdemg` });
            document.body.append(a);
            a.click();
            a.remove();
            URL.revokeObjectURL(url);
        } catch (err) {
            alert(`Export failed: ${err.message}`);
        } finally {
            exportBtn.disabled = false;
            exportBtn.textContent = 'Export';
        }
    }, 'btn-primary');
    sections.push(h('div', { className: 'action-row' }, profileSelect, exportBtn));

    // Import
    const importInput = h('input', { type: 'file', accept: '.mdemg,.json', className: 'file-input' });
    const importBtn = btn('Import', async () => {
        const file = importInput.files?.[0];
        if (!file) { alert('Select a file first'); return; }
        importBtn.disabled = true;
        importBtn.textContent = 'Importing...';
        try {
            const text = await file.text();
            const parsed = JSON.parse(text);
            const chunks = parsed.chunks || [];
            const result = await api.spaceImport({
                space_id: spaceId,
                conflict: 'skip',
                chunks,
            });
            alert(`Import complete: ${result.nodes_created || 0} nodes created, ${result.nodes_skipped || 0} skipped`);
        } catch (err) {
            alert(`Import failed: ${err.message}`);
        } finally {
            importBtn.disabled = false;
            importBtn.textContent = 'Import';
        }
    });
    sections.push(h('div', { className: 'action-row' }, importInput, importBtn));

    clear(container, ...sections);
}
