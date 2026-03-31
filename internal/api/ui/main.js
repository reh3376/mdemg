// main.js — Tab switching, polling orchestration, init
import * as state from './state.js';
import * as api from './api.js';
import { render as renderStatus } from './tabs/status.js';
import { render as renderMemory } from './tabs/memory.js';
import { render as renderLearning } from './tabs/learning.js';
import { render as renderConfig } from './tabs/config.js';
import { render as renderLogs, poll as pollLogsTab } from './tabs/logs.js';
import { render as renderRsic, poll as pollRsicTab } from './tabs/rsic.js';
import { render as renderPlugins } from './tabs/plugins.js';
import { render as renderFeatures } from './tabs/features.js';

const TABS = {
    status:   { render: renderStatus,   label: 'Status' },
    memory:   { render: renderMemory,   label: 'Memory' },
    learning: { render: renderLearning, label: 'Learning' },
    config:   { render: renderConfig,   label: 'Config' },
    logs:     { render: renderLogs,     label: 'Logs' },
    rsic:     { render: renderRsic,     label: 'RSIC' },
    plugins:  { render: renderPlugins,  label: 'Plugins' },
    features: { render: renderFeatures, label: 'Features' },
};

let activeTab = 'status';
let healthTimer = null;
let statsTimer = null;
let logsTimer = null;
let paused = false;

function switchTab(name) {
    if (!TABS[name]) return;
    activeTab = name;

    // Update tab buttons
    document.querySelectorAll('.tab-btn').forEach(b => {
        b.classList.toggle('active', b.dataset.tab === name);
    });

    // Clear stale state subscriptions from the previous tab so they
    // don't overwrite the new tab's content on the next poll cycle.
    state.clearSubscribers();

    // Render tab content
    const content = document.getElementById('content');
    content.innerHTML = '';
    TABS[name].render(content);

    // Re-register persistent (non-tab) subscriptions
    setupSpaceSelector();
}

// Health polling: 10s
async function pollHealth() {
    if (paused) return;
    const [hz, rz, eh] = await Promise.allSettled([
        api.healthz(), api.readyz(), api.embeddingHealth(),
    ]);
    state.batch({
        healthz: hz.status === 'fulfilled' ? hz.value : null,
        readyz: rz.status === 'fulfilled' ? rz.value : null,
        embeddingHealth: eh.status === 'fulfilled' ? eh.value : null,
    });
}

// Stats polling: 30s
async function pollStats() {
    if (paused) return;
    const spaceId = state.get('selectedSpace') || 'mdemg-dev';
    const results = await Promise.allSettled([
        api.memoryStats(spaceId),
        api.learningStats(spaceId),
        api.memoryDistribution(spaceId),
        api.freezeStatus(),
        api.staleEdgeStats(spaceId),
        api.adminSpaces(),
    ]);
    const keys = ['memoryStats', 'learningStats', 'memoryDistribution', 'freezeStatus', 'staleEdgeStats', 'spaces'];
    const updates = {};
    results.forEach((r, i) => {
        updates[keys[i]] = r.status === 'fulfilled' ? r.value : state.get(keys[i]);
    });
    state.batch(updates);
}

// Logs polling: 5s (only when logs tab is active)
async function pollLogs() {
    if (paused || activeTab !== 'logs') return;
    await pollLogsTab();
}

// RSIC polling: on stats interval when RSIC tab is active
async function pollRsic() {
    if (paused || activeTab !== 'rsic') return;
    await pollRsicTab();
}

// Page Visibility API — pause polling when tab is hidden
function setupVisibility() {
    document.addEventListener('visibilitychange', () => {
        paused = document.hidden;
        if (!paused) {
            pollHealth();
            pollStats();
        }
    });
}

// Space selector
function setupSpaceSelector() {
    const select = document.getElementById('space-select');
    if (!select) return;
    state.subscribe('spaces', (spaces) => {
        if (!Array.isArray(spaces)) return;
        const current = state.get('selectedSpace') || 'mdemg-dev';
        select.innerHTML = '';
        for (const sp of spaces) {
            const id = sp.space_id || sp;
            const opt = document.createElement('option');
            opt.value = id;
            opt.textContent = id;
            if (id === current) opt.selected = true;
            select.append(opt);
        }
    });
    select.onchange = () => {
        state.set('selectedSpace', select.value);
        pollStats();
    };
}

// Init
document.addEventListener('DOMContentLoaded', () => {
    // Bind tab buttons
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', () => switchTab(btn.dataset.tab));
    });

    setupSpaceSelector();
    setupVisibility();

    // Initial data fetch
    pollHealth();
    pollStats();

    // Start polling
    healthTimer = setInterval(pollHealth, 10_000);
    statsTimer = setInterval(pollStats, 30_000);
    logsTimer = setInterval(pollLogs, 5_000);
    setInterval(pollRsic, 10_000);

    // Render initial tab
    switchTab('status');
});
