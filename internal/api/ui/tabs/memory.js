// memory.js — Memory tab: stats + knowledge sharing (export/import)
import * as state from '../state.js';
import * as api from '../api.js';
import { h, infoRow, sectionHeader, btn, clear, helpPanel } from '../utils/dom.js';
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
        infoRow('Total Memories', formatNumber(ms.memory_count)),
        // DASHBOARD-FIXES-001: observation_count is in /v1/memory/stats but was never shown.
        infoRow('Observations', formatNumber(ms.observation_count)),
        infoRow('Embedding Coverage', ms.embedding_coverage != null ? `${(ms.embedding_coverage * 100).toFixed(1)}%` : '\u2014'),
        infoRow('Health Score', ms.health_score != null ? `${(ms.health_score * 100).toFixed(1)}%` : '\u2014'),
    ));

    // DASHBOARD-FIXES-001: the memoryDistribution subscription was fetched every
    // 30s and never rendered. Surface the graph phase + edge count + phase alerts.
    const ds = dist?.stats;
    if (ds) {
        sections.push(sectionHeader('Graph Phase'));
        const phaseGroup = h('div', { className: 'info-group' },
            infoRow('Phase', ds.phase || '\u2014'),
            infoRow('Edge Count', formatNumber(ds.edge_count)),
        );
        if (Array.isArray(ds.alerts)) {
            for (const a of ds.alerts) {
                phaseGroup.append(infoRow('Alert', a.description || a.type || ''));
            }
        }
        sections.push(phaseGroup);
    }

    // Layer breakdown
    if (ms.memories_by_layer) {
        sections.push(sectionHeader('By Layer'));
        const maxCount = Math.max(...Object.values(ms.memories_by_layer), 1);
        const layerGroup = h('div', { className: 'info-group' });
        for (const [layer, count] of Object.entries(ms.memories_by_layer)) {
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
    if (ms.temporal_distribution) {
        sections.push(sectionHeader('Temporal Distribution'));
        sections.push(h('div', { className: 'info-group' },
            infoRow('Last 24h', formatNumber(ms.temporal_distribution.last_24h)),
            infoRow('Last 7d', formatNumber(ms.temporal_distribution.last_7d)),
            infoRow('Last 30d', formatNumber(ms.temporal_distribution.last_30d)),
        ));
    }

    // Connectivity
    if (ms.connectivity) {
        sections.push(sectionHeader('Connectivity'));
        sections.push(h('div', { className: 'info-group' },
            infoRow('Avg Degree', ms.connectivity.avg_degree != null ? ms.connectivity.avg_degree.toFixed(1) : '\u2014'),
            infoRow('Max Degree', formatNumber(ms.connectivity.max_degree)),
            infoRow('Orphan Nodes', formatNumber(ms.connectivity.orphan_count)),
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
    sections.push(h('div', { className: 'action-row' }, profileSelect, exportBtn, importInput, importBtn));

    sections.push(helpPanel('Help', [
        { term: 'Total Memories', description: 'Total number of memory nodes in the selected space across all layers (L0\u2013L5).' },
        { term: 'Embedding Coverage', description: 'Percentage of memory nodes that have a vector embedding. Nodes without embeddings cannot participate in semantic retrieval.' },
        { term: 'Health Score', description: 'Overall memory graph health combining embedding coverage, connectivity, and freshness.' },
        { term: 'By Layer', description: 'Bar chart showing memory count per layer. L0=raw observations, L1=summaries, L2=concepts, L3=abstractions, L4=meta-concepts, L5=emergent. Higher layers are formed automatically via Hebbian learning.' },
        { term: 'Temporal Distribution', description: 'How recently memories were created: last 24h, 7d, 30d, or older. Skewed-recent distributions indicate active learning; mostly-older distributions may benefit from fresh ingestion.' },
        { term: 'Connectivity', description: 'Graph connectivity metrics. Avg Degree: mean edges per node. Max Degree: highest edge count on any node. Orphans: nodes with zero edges (isolated, never co-activated).' },
        { term: 'Export', description: 'Download the selected space as a .mdemg file. Profiles: Full (all data), Shareable (excludes secrets), Metadata (structure only), Learned (Hebbian edges only), CMS (conversation observations).' },
        { term: 'Import', description: 'Upload a .mdemg or .json file to import memories into the selected space. Conflict resolution is set to "skip" (existing nodes are not overwritten).' },
    ]));

    clear(container, ...sections);
}
