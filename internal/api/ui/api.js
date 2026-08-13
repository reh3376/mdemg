// api.js — All fetch() calls for the MDEMG browser dashboard

let _baseURL = '';

/** Set the base URL for all API calls (e.g., "http://localhost:10001"). */
export function setBaseURL(url) {
    _baseURL = url.replace(/\/$/, '');
}

/** Get the current base URL. */
export function getBaseURL() { return _baseURL; }

async function get(path) {
    const res = await fetch(_baseURL + path);
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    return res.json();
}

async function patch(path, body) {
    const res = await fetch(_baseURL + path, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    return res.json();
}

async function post(path, body) {
    const opts = {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
    };
    if (body !== null && body !== undefined) {
        opts.body = JSON.stringify(body);
    }
    const res = await fetch(_baseURL + path, opts);
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    return res.json();
}

async function del(path) {
    const res = await fetch(_baseURL + path, { method: 'DELETE' });
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    return res.json();
}

// --- Health (10s polling) ---
export const healthz = () => get('/healthz');
export const readyz = () => get('/readyz');
export const embeddingHealth = () => get('/v1/embedding/health');

// --- Stats (30s polling) ---
export const memoryStats = (spaceId) => get(`/v1/memory/stats?space_id=${encodeURIComponent(spaceId)}`);
export const learningStats = (spaceId) => get(`/v1/learning/stats?space_id=${encodeURIComponent(spaceId)}`);
export const memoryDistribution = (spaceId) => get(`/v1/memory/distribution?space_id=${encodeURIComponent(spaceId)}`);
export const freezeStatus = () => get('/v1/learning/freeze/status');
export const staleEdgeStats = (spaceId) => get(`/v1/memory/edges/stale/stats?space_id=${encodeURIComponent(spaceId)}`);
export const neo4jOverview = () => get('/v1/neo4j/overview');
export const poolMetrics = () => get('/v1/system/pool-metrics');
export const adminSpaces = () => get('/v1/admin/spaces');
export const selfImproveHealth = () => get('/v1/self-improve/health');
export const selfImproveHistory = (limit = 10) => get(`/v1/self-improve/history?limit=${limit}`);
export const selfImproveCalibration = () => get('/v1/self-improve/calibration');

// --- Config (on-demand) ---
export const adminConfig = () => get('/v1/admin/config');
export const updateConfig = (updates) => patch('/v1/admin/config', { updates });

// --- Logs (5s polling) ---
export const adminLogs = (limit = 200) => get(`/v1/admin/logs?limit=${limit}`);

// --- Actions ---
export const triggerCycle = (spaceId, tier = 'meso', dryRun = false) =>
    post('/v1/self-improve/cycle', { space_id: spaceId, tier, dry_run: dryRun });

export const freezeLearning = (spaceId, reason = '', frozenBy = 'ui') =>
    post('/v1/learning/freeze', { space_id: spaceId, reason, frozen_by: frozenBy });

export const unfreezeLearning = (spaceId) =>
    post('/v1/learning/unfreeze', { space_id: spaceId });

// Note: prune uses query param, not JSON body
export const pruneEdges = (spaceId) =>
    post(`/v1/learning/prune?space_id=${encodeURIComponent(spaceId)}`, null);

export const triggerBackup = (spaceId, type = 'partial_space') =>
    post('/v1/backup/trigger', { type, space_ids: [spaceId] });

// --- Backup (on-demand + 5s status polling) ---
export const backupList = (type = '') =>
    get(`/v1/backup/list${type ? `?type=${encodeURIComponent(type)}` : ''}`);
export const backupStatus = (id) => get(`/v1/backup/status/${encodeURIComponent(id)}`);
export const backupManifest = (id) => get(`/v1/backup/manifest/${encodeURIComponent(id)}`);
export const backupDelete = (id) => del(`/v1/backup/${encodeURIComponent(id)}`);
export const backupRestore = (backupId) =>
    post('/v1/backup/restore', { backup_id: backupId });
export const restoreStatus = (id) => get(`/v1/backup/restore/status/${encodeURIComponent(id)}`);

// --- Training Data Export ---
export const trainingDataExport = (req) => post('/v1/training-data/export', req);
export const trainingDataStatus = (id) => get(`/v1/training-data/status/${encodeURIComponent(id)}`);
export const trainingDataDownloadURL = (id) => `${_baseURL}/v1/training-data/download/${encodeURIComponent(id)}`;

export const spaceExport = (spaceId, profile = 'full') =>
    post('/v1/admin/spaces/export', { space_id: spaceId, profile });

export const spaceImport = (data) =>
    post('/v1/admin/spaces/import', data);

// --- RSIC Lifecycle ---
export const rsicStart = () => post('/v1/admin/rsic/start', null);
export const rsicStop = () => post('/v1/admin/rsic/stop', null);
export const rsicRestart = () => post('/v1/admin/rsic/restart', null);

// --- Plugins ---
export const pluginList = () => get('/v1/plugins');
export const pluginDetail = (id) => get(`/v1/plugins/${encodeURIComponent(id)}`);
export const pluginStart = (id) => post(`/v1/plugins/${encodeURIComponent(id)}/start`, null);
export const pluginStop = (id) => post(`/v1/plugins/${encodeURIComponent(id)}/stop`, null);
export const pluginRestart = (id) => post(`/v1/plugins/${encodeURIComponent(id)}/restart`, null);
export const pluginValidate = (id) => post(`/v1/plugins/${encodeURIComponent(id)}/validate`, null);

// --- Features ---
export const featureList = () => get('/v1/admin/features');
export const featureStart = (name) => post('/v1/admin/features/start', { name });
export const featureStop = (name) => post('/v1/admin/features/stop', { name });
export const featureRestart = (name) => post('/v1/admin/features/restart', { name });

// --- Review (HITL-REVIEW-001) ---
export const reviewDatasets = (spaceId) => get(`/v1/review/datasets?space_id=${encodeURIComponent(spaceId)}`);
export const reviewNext = (datasetId, spaceId) => get(`/v1/review/next?dataset_id=${encodeURIComponent(datasetId)}&space_id=${encodeURIComponent(spaceId)}`);
export const reviewGrade = (body) => post('/v1/review/grade', body);
export const reviewReverse = (gradeId) => post('/v1/review/reverse', { grade_id: gradeId });
// HITL-AUTOGRADE-PREVIEW-001: get the autograder's proposed grades for an
// item WITHOUT recording. UI pre-fills rubric radios from the response so
// operator hits `space` to accept-as-is or `0-4` to override per-dimension.
export const reviewAutogradePreview = (datasetId, itemId, spaceId) =>
    post('/v1/review/autograde-preview', { dataset_id: datasetId, item_id: itemId, space_id: spaceId });

// --- Server ---
export const serverRestart = () => post('/v1/admin/restart', null);

// --- Instance Discovery ---
// Always fetches from current host (ignores _baseURL) since discovery is local-only.
export const adminInstances = () => fetch('/v1/admin/instances').then(r => {
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`);
    return r.json();
});

// JIMINY-MODE-001 — read/write Jiminy enforcement mode
export const jiminyStrictGet = (sessionID) => get(`/v1/jiminy/strict?session_id=${encodeURIComponent(sessionID)}`);
export const jiminyStrictSet = (sessionID, enabled) => post('/v1/jiminy/strict', { session_id: sessionID, enabled });

// JIMINY-ENFORCE-003 / ENFORCE-UI-OVERRIDES (2026-08-03) — override endpoints
export const jiminyOverrideList = (sessionID) => {
    const q = sessionID ? `?session_id=${encodeURIComponent(sessionID)}` : '';
    return get(`/v1/jiminy/override${q}`);
};
export const jiminyOverrideApply = (sessionID, constraintCode, reason, durationSec) =>
    post('/v1/jiminy/override', { session_id: sessionID, constraint_code: constraintCode, reason, duration_sec: durationSec });
export const jiminyOverrideRevoke = async (sessionID, constraintCode) => {
    const res = await fetch(_baseURL + '/v1/jiminy/override', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ session_id: sessionID, constraint_code: constraintCode }),
    });
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    return res.json();
};
export const jiminyOverrideHistory = (spaceID, hours) => {
    const q = new URLSearchParams();
    if (spaceID) q.set('space_id', spaceID);
    if (hours) q.set('hours', String(hours));
    return get('/v1/jiminy/override/history' + (q.toString() ? '?' + q.toString() : ''));
};
