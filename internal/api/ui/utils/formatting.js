// formatting.js — Number and time formatting utilities

export function formatNumber(n) {
    if (n == null) return '\u2014';
    return new Intl.NumberFormat().format(n);
}

export function formatUptime(startTimeISO) {
    if (!startTimeISO) return '\u2014';
    const ms = Date.now() - new Date(startTimeISO).getTime();
    const s = Math.floor(ms / 1000);
    if (s < 60) return `${s}s`;
    if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`;
    const hrs = Math.floor(s / 3600);
    const mins = Math.floor((s % 3600) / 60);
    return `${hrs}h ${mins}m`;
}

export function timeAgo(isoStr) {
    if (!isoStr) return '\u2014';
    const sec = Math.floor((Date.now() - new Date(isoStr).getTime()) / 1000);
    if (sec < 0) return 'just now';
    if (sec < 60) return 'just now';
    if (sec < 3600) return `${Math.floor(sec / 60)}m ago`;
    if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`;
    return `${Math.floor(sec / 86400)}d ago`;
}

export function formatBytes(bytes) {
    if (bytes == null) return '\u2014';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`;
    return `${(bytes / 1073741824).toFixed(1)} GB`;
}

export function pct(n) {
    if (n == null) return '\u2014';
    return `${(n * 100).toFixed(1)}%`;
}
