// dom.js — DOM creation utilities

export function h(tag, attrs = {}, ...children) {
    const el = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
        if (k === 'className') el.className = v;
        else if (k.startsWith('on') && typeof v === 'function') el[k] = v;
        else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
        else el.setAttribute(k, v);
    }
    for (const child of children.flat()) {
        if (child == null) continue;
        el.append(typeof child === 'string' ? document.createTextNode(child) : child);
    }
    return el;
}

export function infoRow(label, value) {
    return h('div', { className: 'info-row' },
        h('span', { className: 'info-label' }, label),
        h('span', { className: 'info-value' }, String(value ?? '\u2014')),
    );
}

export function sectionHeader(title) {
    return h('h3', { className: 'section-header' }, title);
}

export function statusBadge(status) {
    const s = String(status).toLowerCase();
    const cls = (s === 'ok' || s === 'healthy' || s === 'ready' || s === 'active')
        ? 'badge-ok' : (s === 'degraded' || s === 'warn')
        ? 'badge-warn' : 'badge-err';
    return h('span', { className: `badge ${cls}` }, status);
}

export function btn(label, onclick, className = '') {
    return h('button', { className: `btn ${className}`.trim(), onclick }, label);
}

export function clear(container, ...children) {
    container.replaceChildren(...children);
}

// Help wiki panel — toggled by a "?" button in the tab header.
// entries: array of { term, description } objects.
export function helpPanel(title, entries) {
    const content = h('div', { className: 'help-content', style: { display: 'none' } },
        ...entries.map(e =>
            h('div', { className: 'help-entry' },
                h('dt', { className: 'help-term' }, e.term),
                h('dd', { className: 'help-desc' }, e.description),
            )
        ),
    );
    const toggle = h('button', {
        className: 'btn help-toggle',
        title: 'Toggle help wiki',
        onclick: () => {
            const vis = content.style.display === 'none';
            content.style.display = vis ? 'block' : 'none';
            toggle.textContent = vis ? 'Hide Help' : '? Help';
        },
    }, '? Help');
    return h('div', { className: 'help-panel' },
        h('div', { className: 'help-header' },
            sectionHeader(title),
            toggle,
        ),
        content,
    );
}
