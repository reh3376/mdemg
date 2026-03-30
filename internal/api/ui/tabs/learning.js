// learning.js — Learning tab: Hebbian stats + freeze/unfreeze/prune
import * as state from '../state.js';
import * as api from '../api.js';
import { h, infoRow, sectionHeader, statusBadge, btn, clear, helpPanel } from '../utils/dom.js';
import { formatNumber } from '../utils/formatting.js';

let container;

export function render(el) {
    container = el;
    clear(container, h('p', { className: 'muted' }, 'Loading learning stats...'));
    state.subscribe('learningStats', () => update());
    state.subscribe('freezeStatus', () => update());
}

function update() {
    const ls = state.get('learningStats');
    if (!container || !ls) return;
    const fs = state.get('freezeStatus');

    const sections = [];
    const spaceId = state.get('selectedSpace') || 'mdemg-dev';

    // Edge stats
    sections.push(sectionHeader('Hebbian Learning Edges'));
    sections.push(h('div', { className: 'info-group' },
        infoRow('Total Edges', formatNumber(ls.total_edges)),
        infoRow('Co-Activated', formatNumber(ls.co_activated)),
        infoRow('Strong', formatNumber(ls.strong)),
        infoRow('Surprising', formatNumber(ls.surprising)),
        infoRow('Below Threshold', formatNumber(ls.below_threshold)),
        infoRow('Avg Weight', ls.avg_weight != null ? ls.avg_weight.toFixed(3) : '\u2014'),
        infoRow('Max Weight', ls.max_weight != null ? ls.max_weight.toFixed(3) : '\u2014'),
        infoRow('Avg Days Inactive', ls.avg_days_inactive != null ? ls.avg_days_inactive.toFixed(1) : '\u2014'),
    ));

    // Config
    if (ls.config) {
        sections.push(sectionHeader('Configuration'));
        sections.push(h('div', { className: 'info-group' },
            infoRow('Decay/Day', ls.config.decay_per_day),
            infoRow('Prune Threshold', ls.config.prune_threshold),
            infoRow('Max Edges/Node', ls.config.max_edges_per_node),
        ));
    }

    // Freeze state
    sections.push(sectionHeader('Freeze State'));
    const isFrozen = fs?.frozen || false;
    const freezeBadge = statusBadge(isFrozen ? 'frozen' : 'active');
    const freezeRow = infoRow('State', '');
    freezeRow.querySelector('.info-value').replaceChildren(freezeBadge);
    sections.push(h('div', { className: 'info-group' },
        freezeRow,
        ...(isFrozen ? [
            infoRow('Frozen At', fs.frozen_at || '\u2014'),
            infoRow('Frozen By', fs.frozen_by || '\u2014'),
            infoRow('Reason', fs.reason || '\u2014'),
        ] : []),
    ));

    // Actions
    sections.push(sectionHeader('Actions'));
    const actionRow = h('div', { className: 'action-row' });

    if (!isFrozen) {
        actionRow.append(btn('Freeze', async () => {
            const reason = prompt('Freeze reason (optional):') ?? '';
            try {
                await api.freezeLearning(spaceId, reason);
                state.set('freezeStatus', { frozen: true, reason, frozen_by: 'ui', frozen_at: new Date().toISOString() });
            } catch (err) { alert(`Freeze failed: ${err.message}`); }
        }));
    } else {
        actionRow.append(btn('Unfreeze', async () => {
            try {
                await api.unfreezeLearning(spaceId);
                state.set('freezeStatus', { frozen: false });
            } catch (err) { alert(`Unfreeze failed: ${err.message}`); }
        }));
    }

    actionRow.append(btn('Prune', async () => {
        if (!confirm('Prune stale learning edges? This cannot be undone.')) return;
        try {
            const result = await api.pruneEdges(spaceId);
            alert(`Pruned ${result.deleted || 0} edges`);
        } catch (err) { alert(`Prune failed: ${err.message}`); }
    }, 'btn-danger'));

    sections.push(actionRow);

    sections.push(helpPanel('Help', [
        { term: 'Total Edges', description: 'Total Hebbian learning edges in the graph. Edges form when two memory nodes are co-activated (referenced together) and strengthen with repeated co-activation.' },
        { term: 'Co-Activated', description: 'Edges where both connected nodes were activated in the same context. These represent learned associations.' },
        { term: 'Strong', description: 'Edges with weight above the strong threshold. Strong edges indicate reliable, frequently reinforced associations.' },
        { term: 'Surprising', description: 'Edges connecting nodes that are semantically distant but frequently co-activated. These represent novel, non-obvious associations \u2014 often the most valuable discoveries.' },
        { term: 'Below Threshold', description: 'Edges with weight below the prune threshold. These are candidates for pruning \u2014 weak associations that may be noise.' },
        { term: 'Avg/Max Weight', description: 'Edge weight statistics. Weights range from 0 to 1. Higher weight = stronger association. Weights decay daily by the configured decay rate.' },
        { term: 'Avg Days Inactive', description: 'Mean number of days since edges were last co-activated. High values indicate the graph has stale associations that may benefit from pruning.' },
        { term: 'Decay/Day', description: 'Daily decay rate applied to all edge weights. Default 0.01 = 1% per day. Ensures unused associations fade over time.' },
        { term: 'Prune Threshold', description: 'Weight below which edges are eligible for pruning. Default 0.05.' },
        { term: 'Max Edges/Node', description: 'Maximum Hebbian edges per node. When exceeded, weakest edges are pruned automatically.' },
        { term: 'Freeze', description: 'Pauses all Hebbian learning. No new edges are created and no weights are updated while frozen. Use during data imports or maintenance.' },
        { term: 'Unfreeze', description: 'Resumes Hebbian learning after a freeze.' },
        { term: 'Prune', description: 'Permanently deletes all edges below the prune threshold. This cannot be undone. Reduces graph noise and improves retrieval precision.' },
    ]));

    clear(container, ...sections);
}
