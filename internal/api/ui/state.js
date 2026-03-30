// state.js — Minimal pub/sub reactive state store
const state = {};
const subscribers = {};

export function get(key) {
    return state[key];
}

export function set(key, value) {
    state[key] = value;
    (subscribers[key] || []).forEach(fn => fn(value));
}

export function subscribe(key, fn) {
    (subscribers[key] ??= []).push(fn);
    if (key in state) fn(state[key]);
    return () => { subscribers[key] = subscribers[key].filter(f => f !== fn); };
}

export function batch(updates) {
    for (const [k, v] of Object.entries(updates)) {
        state[k] = v;
    }
    for (const k of Object.keys(updates)) {
        (subscribers[k] || []).forEach(fn => fn(state[k]));
    }
}

// Clear all subscribers — call before rendering a new tab to prevent
// stale callbacks from overwriting the active tab's content.
export function clearSubscribers() {
    for (const key of Object.keys(subscribers)) {
        subscribers[key] = [];
    }
}
