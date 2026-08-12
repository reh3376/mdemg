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

// JIMINY-HITL-VELOCITY-001 (2026-08-12): keyboard-driven fast mode.
let keyHandlerBound = false;
let currentDimensionInputs = null; // set by renderItem; used by keyboard handler to pre-fill all dims
let autoAdvanceTimer = null;

function spaceId() { return state.get('selectedSpace') || 'mdemg-dev'; }

export function render(el) {
    container = el;
    clear(container, sectionHeader('Human-in-the-Loop Review'), h('p', { className: 'muted' }, 'Loading datasets…'));
    bindKeyboardHandler();
    loadDatasets();
}

// JIMINY-HITL-VELOCITY-001: keyboard-driven bulk review. Idempotent — safe
// to call on every tab render.
function bindKeyboardHandler() {
    if (keyHandlerBound) return;
    document.addEventListener('keydown', handleReviewKey);
    keyHandlerBound = true;
}

function isReviewTabActive() {
    // Only fire when the review container is in the DOM AND visible.
    if (!container || !container.isConnected) return false;
    // If a text field / textarea has focus, defer to normal typing behavior.
    const ae = document.activeElement;
    if (ae && (ae.tagName === 'INPUT' && ae.type !== 'radio' && ae.type !== 'checkbox' || ae.tagName === 'TEXTAREA')) {
        return false;
    }
    return true;
}

function setAllDimensions(value) {
    if (!currentDimensionInputs || !currentRubric) return false;
    let count = 0;
    for (const dim of (currentRubric.dimensions || [])) {
        const radios = container.querySelectorAll(`input[type="radio"][name="${dim.key}"]`);
        for (const r of radios) {
            if (r.value === String(value)) {
                r.checked = true;
                currentDimensionInputs[dim.key] = value;
                count++;
                break;
            }
        }
    }
    return count > 0;
}

function clickIfPresent(id) {
    const el = document.getElementById(id);
    if (!el) return false;
    el.click();
    return true;
}

function toggleHelpOverlay() {
    const existing = document.getElementById('review-help-overlay');
    if (existing) { existing.remove(); return; }
    const overlay = h('div', {
        id: 'review-help-overlay',
        style: {
            position: 'fixed', top: '20%', left: '50%', transform: 'translateX(-50%)',
            background: '#1a1a1a', border: '1px solid #4a90e2', padding: '20px', borderRadius: '6px',
            zIndex: '9999', fontSize: '13px', lineHeight: '1.6', color: '#e0e0e0',
            boxShadow: '0 4px 20px rgba(0,0,0,0.5)', maxWidth: '480px',
        },
    },
        h('h4', { style: { margin: '0 0 12px', color: '#4a90e2' } }, 'Keyboard shortcuts (Review tab)'),
        h('div', {},
            h('kbd', {}, '0'), '..', h('kbd', {}, '4'), ' — set every rubric dimension to that value', h('br'),
            h('kbd', {}, 'Space'), ' / ', h('kbd', {}, 'Enter'), ' — submit the current grade', h('br'),
            h('kbd', {}, 'n'), ' — load next item (works post-submit)', h('br'),
            h('kbd', {}, 'u'), ' — reverse (undo) the last grade (works post-submit, before auto-advance)', h('br'),
            h('kbd', {}, '?'), ' — toggle this help', h('br'),
            h('kbd', {}, 'Esc'), ' — close this help / cancel auto-advance'),
        h('div', { style: { marginTop: '12px', fontSize: '11px', color: '#888' } },
            'Fast-mode tip: read the item → press a number key → space → auto-advance. Grade differ per-dimension by clicking a radio before pressing space. Text fields (suggested-guidance box) do not intercept shortcuts.'));
    document.body.append(overlay);
}

function cancelAutoAdvance() {
    if (autoAdvanceTimer) { clearTimeout(autoAdvanceTimer); autoAdvanceTimer = null; }
}

function handleReviewKey(e) {
    if (!isReviewTabActive()) return;
    // '?' toggles help regardless of shift key state.
    if (e.key === '?' || (e.key === '/' && e.shiftKey)) {
        e.preventDefault(); toggleHelpOverlay(); return;
    }
    if (e.key === 'Escape') {
        const overlay = document.getElementById('review-help-overlay');
        if (overlay) { overlay.remove(); e.preventDefault(); return; }
        if (autoAdvanceTimer) { cancelAutoAdvance(); e.preventDefault(); return; }
    }
    // Number keys 0-4 set all dimensions to that value.
    if (e.key >= '0' && e.key <= '4' && !e.ctrlKey && !e.metaKey && !e.altKey) {
        if (setAllDimensions(parseInt(e.key, 10))) { e.preventDefault(); return; }
    }
    // Space / Enter submits.
    if ((e.key === ' ' || e.key === 'Enter') && !e.ctrlKey && !e.metaKey) {
        // Only if we have an active item (not on post-submit state).
        if (currentDimensionInputs) {
            e.preventDefault();
            const btns = container.querySelectorAll('button');
            for (const b of btns) {
                if (b.textContent === 'Submit grade') { b.click(); return; }
            }
        }
    }
    // 'n' loads next item.
    if (e.key === 'n' && !e.ctrlKey && !e.metaKey && !e.altKey) {
        cancelAutoAdvance();
        const btns = container.querySelectorAll('button');
        for (const b of btns) {
            if (b.textContent && b.textContent.startsWith('Next item')) { e.preventDefault(); b.click(); return; }
        }
        // Fallback: if no next-button visible, load directly.
        if (currentDataset) { e.preventDefault(); loadNext(); }
    }
    // 'u' reverses last grade.
    if (e.key === 'u' && !e.ctrlKey && !e.metaKey && !e.altKey) {
        cancelAutoAdvance();
        const btns = container.querySelectorAll('button');
        for (const b of btns) {
            if (b.textContent === 'Reverse last grade') { e.preventDefault(); b.click(); return; }
        }
    }
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
    cancelAutoAdvance();
    const work = document.getElementById('review-work');
    work.innerHTML = '';
    const it = currentItem;
    const m = it.meta || {};
    // The auto-classifier's full picture — the thing the SME judges in
    // outcome_label_correctness: what it decided, how confident, how it decided.
    const metaRow = (k, v) => h('div', {},
        h('span', { style: { color: '#888', display: 'inline-block', minWidth: '140px' } }, k), h('span', {}, v || '—'));
    const remaining = (datasets.find(d => d.id === currentDataset) || {}).candidate_count;
    // JIMINY-HITL-VELOCITY-001: prominent session counter (velocity feedback)
    work.append(h('div', {
        style: { padding: '6px 10px', margin: '0 0 8px', background: '#1a2c1a', border: '1px solid #4a8', borderRadius: '4px', color: '#8fc', fontFamily: 'monospace', fontSize: '13px' },
    }, `Session graded: ${sessionGraded}   ·   Shortcuts: 0-4 grade · space submit · n next · u undo · ? help`));
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
    currentDimensionInputs = inputs; // JIMINY-HITL-VELOCITY-001: expose to keyboard handler
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
                status.textContent = `✓ graded ${r.grade_id} · gold ${r.gold_score} · reinforced ${r.reinforcement_applied}   (auto-advance in 400ms — press u to reverse, n to advance now, esc to cancel)`;
                currentDimensionInputs = null; // gate keyboard 'space' until next item loads
                renderAfter();
                // JIMINY-HITL-VELOCITY-001: auto-advance for keyboard-only bulk flow
                cancelAutoAdvance();
                autoAdvanceTimer = setTimeout(() => { autoAdvanceTimer = null; loadNext(); }, 400);
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
