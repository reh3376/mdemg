// review.js — Human-in-the-Loop Review tab (HITL-REVIEW-001).
// Functional developer tool: pick a dataset, grade items against the rubric,
// optionally reinforce the live system (reversible). Not polished by design.
import * as api from '../api.js';
import * as state from '../state.js';
import { h, sectionHeader, btn, clear } from '../utils/dom.js';

let container;
let datasets = [];
let currentDataset = null;
let currentItem = null;
let currentRubric = null;
let lastGradeId = null;
let suggestedEl = null;
let reinforceDefault = false;
let sessionGraded = 0;

function spaceId() { return state.get('selectedSpace') || 'mdemg-dev'; }

export function render(el) {
    container = el;
    clear(container, sectionHeader('Human-in-the-Loop Review'), h('p', { className: 'muted' }, 'Loading datasets…'));
    loadDatasets();
}

async function loadDatasets() {
    try {
        const data = (await api.reviewDatasets(spaceId())).data || {};
        datasets = data.datasets || [];
        reinforceDefault = !!data.reinforce_default;
        renderShell(data.current_rubric_version);
    } catch (err) {
        clear(container, sectionHeader('Human-in-the-Loop Review'),
            h('p', { className: 'error', id: 'review-error' },
                `Failed to load datasets: ${err.message} (review may be disabled, or this surface is admin-gated)`));
    }
}

function renderShell(rubricVersion) {
    const picker = h('select', { id: 'review-dataset', onchange: (e) => selectDataset(e.target.value) },
        h('option', { value: '' }, '— pick a dataset —'),
        ...datasets.map(d => h('option', { value: d.id }, `${d.display_name} (${d.candidate_count} to review)`)));
    clear(container,
        sectionHeader('Human-in-the-Loop Review'),
        h('div', { className: 'muted' }, `Rubric ${rubricVersion || 'gr-v1'} — grade items, optionally reinforce the live system (reversible).`),
        h('div', { style: { margin: '12px 0' } }, 'Dataset: ', picker),
        h('div', { id: 'review-dataset-desc', style: { margin: '4px 0 12px', fontSize: '12px', color: '#9bb9d6', minHeight: '1em' } }),
        h('div', { id: 'review-work' }));
}

// Show the selected dataset's purpose ('more info').
function showDatasetDesc(id) {
    const el = document.getElementById('review-dataset-desc');
    if (!el) return;
    el.innerHTML = '';
    const d = datasets.find(x => x.id === id);
    if (d && d.description) {
        el.append(h('span', { title: 'what this dataset is + what you are judging' }, 'ⓘ '), d.description);
    }
}

async function selectDataset(id) {
    currentDataset = id || null;
    showDatasetDesc(id);
    const work = document.getElementById('review-work');
    if (work) work.innerHTML = '';
    if (id) await loadNext();
}

async function loadNext() {
    const work = document.getElementById('review-work');
    work.innerHTML = '';
    work.append(h('p', { className: 'muted' }, 'Fetching next item…'));
    try {
        const data = (await api.reviewNext(currentDataset, spaceId())).data || {};
        currentItem = data.item;
        currentRubric = data.rubric;
        if (!currentItem) {
            work.innerHTML = '';
            work.append(h('p', { className: 'muted', id: 'review-empty' }, data.message || 'No items to review.'));
            return;
        }
        renderItem();
    } catch (err) {
        work.innerHTML = '';
        work.append(h('p', { className: 'error' }, err.message));
    }
}

function renderItem() {
    const work = document.getElementById('review-work');
    work.innerHTML = '';
    const it = currentItem;
    const m = it.meta || {};
    // The auto-classifier's full picture — the thing the SME judges in
    // outcome_label_correctness: what it decided, how confident, how it decided.
    const metaRow = (k, v) => h('div', {},
        h('span', { style: { color: '#888', display: 'inline-block', minWidth: '140px' } }, k), h('span', {}, v || '—'));
    const remaining = (datasets.find(d => d.id === currentDataset) || {}).candidate_count;
    work.append(h('div', { className: 'card', id: 'review-item' },
        h('div', { className: 'muted', id: 'review-progress' },
            `item ${it.item_id} · recorded ${m.recorded_at || '?'} · ${sessionGraded} graded this session` +
            (remaining != null ? ` · ~${remaining} remaining` : '')),
        h('h4', {}, 'Auto-classifier verdict (judge this)'),
        h('div', { style: { background: '#1c1c1c', padding: '8px', borderRadius: '4px', marginBottom: '8px' } },
            metaRow('verdict', it.auto_label),
            metaRow('confidence (similarity)', m.similarity),
            metaRow('labeled via', m.classifier_source)),
        h('h4', {}, `Surfaced guidance  ·  type: ${m.guidance_type || '?'}`),
        h('pre', {}, it.content || '(empty)'),
        h('h4', {}, 'Agent action (what the agent actually did)'),
        h('pre', {}, it.context || '(none)'),
        h('h4', {}, 'Provenance'),
        h('div', { style: { fontSize: '12px' } },
            metaRow('constraint code', m.constraint_code),
            metaRow('source role / layer', `${m.source_role_type || '—'} / L${m.source_layer || '?'}`),
            metaRow('session', m.session_id),
            metaRow('guidance id', m.guidance_id))));

    const form = h('div', { className: 'card', id: 'review-form' });
    const inputs = {};
    for (const dim of (currentRubric && currentRubric.dimensions) || []) {
        const radios = h('div', {});
        for (let lvl = 0; lvl <= 4; lvl++) {
            radios.append(h('label', { style: { marginRight: '10px', fontSize: '12px', display: 'inline-block' } },
                h('input', {
                    type: 'radio', name: dim.key, value: String(lvl),
                    onchange: () => { inputs[dim.key] = lvl; },
                }), ` ${lvl}: ${dim.anchors[lvl]}`));
        }
        form.append(h('div', { style: { margin: '8px 0' } }, h('strong', {}, dim.key), radios));
    }
    // SME corrective capture: "what would have been better guidance?" — the
    // highest-value training signal for the synthesis retrain (gold actionable,
    // task-specific guidance authored by a human).
    suggestedEl = h('textarea', {
        id: 'review-suggested', rows: '3',
        placeholder: 'Optional: what would have been BETTER guidance here? (a specific, actionable, task-relevant directive — becomes a gold training example)',
        style: { width: '100%', boxSizing: 'border-box', fontFamily: 'monospace', fontSize: '12px' },
    });
    form.append(h('div', { style: { margin: '8px 0' } },
        h('strong', {}, 'Suggested better guidance'), suggestedEl));

    const reinforce = h('input', { type: 'checkbox', id: 'review-reinforce', checked: reinforceDefault });
    form.append(h('div', { style: { margin: '8px 0' } },
        h('label', {}, reinforce, ' reinforce the live system (reversible)')));

    const status = h('div', { className: 'muted', id: 'review-status' });
    form.append(h('div', {},
        btn('Preview (dry-run)', async () => {
            status.textContent = 'previewing…';
            try {
                const r = (await api.reviewGrade(gradeBody(inputs, true, reinforce.checked))).data;
                status.textContent = 'PREVIEW: ' + ((r.preview && r.preview.summary) || `gold ${r.gold_score}`);
            } catch (e) { status.textContent = 'error: ' + e.message; }
        }),
        ' ',
        btn('Submit grade', async () => {
            status.textContent = 'submitting…';
            try {
                const r = (await api.reviewGrade(gradeBody(inputs, false, reinforce.checked))).data;
                lastGradeId = r.grade_id;
                sessionGraded += 1;
                status.textContent = `✓ graded ${r.grade_id} · gold ${r.gold_score} · reinforced ${r.reinforcement_applied}`;
                renderAfter();
            } catch (e) { status.textContent = 'error: ' + e.message; }
        }, 'primary'),
        status));
    work.append(form);
}

function gradeBody(inputs, dryRun, reinforce) {
    return {
        dataset_id: currentDataset, item_id: currentItem.item_id, space_id: spaceId(),
        grader_id: 'ui', dimensions: inputs, reinforce, dry_run: dryRun,
        suggested_guidance: suggestedEl ? suggestedEl.value : '',
    };
}

function renderAfter() {
    const work = document.getElementById('review-work');
    const revStatus = h('span', { className: 'muted', id: 'review-reverse-status', style: { marginLeft: '8px' } });
    work.append(h('div', { id: 'review-after', style: { marginTop: '10px' } },
        btn('Reverse last grade', async () => {
            revStatus.textContent = 'reversing…';
            try { await api.reviewReverse(lastGradeId); revStatus.textContent = `↩ reversed ${lastGradeId} (live state restored)`; }
            catch (e) { revStatus.textContent = 'error: ' + e.message; }
        }),
        ' ',
        btn('Next item →', () => loadNext(), 'primary'),
        revStatus));
}
