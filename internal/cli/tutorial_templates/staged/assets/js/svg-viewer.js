// ONBOARDING-TUTORIAL-ENDUSER-001 interactive SVG viewer (2026-10-26).
//
// Every <figure class="svg-figure"> in the deck becomes an interactive
// viewer at page load:
//   * default: SVG fits entirely inside a fixed-size container
//   * mouse wheel = zoom at cursor
//   * click-drag = pan when zoomed
//   * double-click or ⟳ button = reset to fit
//   * +, – buttons = zoom step
//   * ⤢ = fullscreen the figure
//   * touch: pinch-to-zoom + single-finger pan
//
// Decouples diagram display from reveal.js's slide-tile size constraints,
// so every converted SVG (tables, flowcharts, anything else) can be
// examined at any detail level without the slide-size wall.
//
// Vanilla JS, no external dependency. Idempotent (re-scans on hashchange
// + MutationObserver for lazily-injected content).

(function () {
  'use strict';

  var MIN_SCALE = 1;    // scale 1 = fit-to-container (never zoom out past fit)
  var MAX_SCALE = 10;   // 10x zoom covers detail in the densest diagrams
  var WHEEL_STEP = 1.15;
  var BUTTON_STEP = 1.3;

  function create(tag, cls, attrs, text) {
    var el = document.createElement(tag);
    if (cls) el.className = cls;
    if (attrs) for (var k in attrs) el.setAttribute(k, attrs[k]);
    if (text != null) el.textContent = text;
    return el;
  }

  function initViewer(figure) {
    if (!figure || figure.dataset.viewerReady === '1') return;
    var img = figure.querySelector('img');
    if (!img) return;
    figure.dataset.viewerReady = '1';

    // Toolbar
    var toolbar = create('div', 'svg-viewer-toolbar', { role: 'toolbar', 'aria-label': 'Diagram controls' });
    var btnIn = create('button', null, { type: 'button', 'data-act': 'zoomin', 'aria-label': 'Zoom in', title: 'Zoom in' }, '+');
    var btnOut = create('button', null, { type: 'button', 'data-act': 'zoomout', 'aria-label': 'Zoom out', title: 'Zoom out' }, '−');
    var btnReset = create('button', null, { type: 'button', 'data-act': 'reset', 'aria-label': 'Reset view', title: 'Reset to fit (dbl-click)' }, '⟲');
    var btnFs = create('button', null, { type: 'button', 'data-act': 'fullscreen', 'aria-label': 'Fullscreen', title: 'Fullscreen' }, '⛶');
    toolbar.appendChild(btnIn);
    toolbar.appendChild(btnOut);
    toolbar.appendChild(btnReset);
    toolbar.appendChild(btnFs);
    figure.appendChild(toolbar);

    // Hint shown on first hover ever (dismissed via localStorage)
    var hint = create('div', 'svg-viewer-hint', { 'aria-hidden': 'true' }, 'scroll to zoom · drag to pan · dbl-click to reset');
    figure.appendChild(hint);

    var state = { scale: 1, tx: 0, ty: 0 };

    function apply() {
      img.style.transform = 'translate(' + state.tx + 'px, ' + state.ty + 'px) scale(' + state.scale + ')';
    }

    function clamp() {
      // Constrain translate so image never panned entirely out of frame.
      var rect = figure.getBoundingClientRect();
      var excess = Math.max(0, (state.scale - 1) * 0.5);
      var maxX = rect.width * excess;
      var maxY = rect.height * excess;
      if (state.tx > maxX) state.tx = maxX;
      if (state.tx < -maxX) state.tx = -maxX;
      if (state.ty > maxY) state.ty = maxY;
      if (state.ty < -maxY) state.ty = -maxY;
    }

    function reset() {
      state.scale = 1; state.tx = 0; state.ty = 0;
      apply();
    }

    function zoomAt(factor, clientX, clientY) {
      var rect = figure.getBoundingClientRect();
      var cx = clientX != null ? (clientX - rect.left - rect.width / 2) : 0;
      var cy = clientY != null ? (clientY - rect.top - rect.height / 2) : 0;
      var old = state.scale;
      var next = Math.min(MAX_SCALE, Math.max(MIN_SCALE, old * factor));
      if (next === old) return;
      // Preserve point under cursor: translate so (cx, cy) stays stationary.
      var ratio = next / old;
      state.tx = cx - (cx - state.tx) * ratio;
      state.ty = cy - (cy - state.ty) * ratio;
      state.scale = next;
      clamp();
      apply();
    }

    // Mouse wheel = zoom at cursor
    figure.addEventListener('wheel', function (e) {
      e.preventDefault();
      var factor = e.deltaY < 0 ? WHEEL_STEP : 1 / WHEEL_STEP;
      zoomAt(factor, e.clientX, e.clientY);
    }, { passive: false });

    // Mouse drag = pan
    var dragging = false, lastX = 0, lastY = 0;
    figure.addEventListener('mousedown', function (e) {
      if (e.target.closest('.svg-viewer-toolbar')) return;
      dragging = true; lastX = e.clientX; lastY = e.clientY;
      figure.classList.add('svg-viewer-grabbing');
      e.preventDefault();
    });
    document.addEventListener('mousemove', function (e) {
      if (!dragging) return;
      state.tx += e.clientX - lastX;
      state.ty += e.clientY - lastY;
      lastX = e.clientX; lastY = e.clientY;
      clamp(); apply();
    });
    document.addEventListener('mouseup', function () {
      if (!dragging) return;
      dragging = false;
      figure.classList.remove('svg-viewer-grabbing');
    });

    // Double-click = reset
    figure.addEventListener('dblclick', function (e) {
      if (e.target.closest('.svg-viewer-toolbar')) return;
      reset();
    });

    // Touch: pinch zoom + single-finger pan
    var pinchDist = 0, pinchScale = 1, pinchMidX = 0, pinchMidY = 0;
    figure.addEventListener('touchstart', function (e) {
      if (e.touches.length === 2) {
        var a = e.touches[0], b = e.touches[1];
        pinchDist = Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY);
        pinchScale = state.scale;
        pinchMidX = (a.clientX + b.clientX) / 2;
        pinchMidY = (a.clientY + b.clientY) / 2;
      } else if (e.touches.length === 1) {
        dragging = true;
        lastX = e.touches[0].clientX;
        lastY = e.touches[0].clientY;
      }
    }, { passive: true });
    figure.addEventListener('touchmove', function (e) {
      if (e.touches.length === 2) {
        e.preventDefault();
        var a = e.touches[0], b = e.touches[1];
        var d = Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY);
        if (pinchDist > 0) {
          var factor = d / pinchDist;
          var oldS = state.scale;
          state.scale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, pinchScale * factor));
          var rect = figure.getBoundingClientRect();
          var cx = pinchMidX - rect.left - rect.width / 2;
          var cy = pinchMidY - rect.top - rect.height / 2;
          var ratio = state.scale / oldS;
          state.tx = cx - (cx - state.tx) * ratio;
          state.ty = cy - (cy - state.ty) * ratio;
          clamp(); apply();
        }
      } else if (dragging && e.touches.length === 1) {
        e.preventDefault();
        state.tx += e.touches[0].clientX - lastX;
        state.ty += e.touches[0].clientY - lastY;
        lastX = e.touches[0].clientX;
        lastY = e.touches[0].clientY;
        clamp(); apply();
      }
    }, { passive: false });
    figure.addEventListener('touchend', function () {
      dragging = false;
      pinchDist = 0;
    });

    // Toolbar button actions
    toolbar.addEventListener('click', function (e) {
      var btn = e.target.closest('button');
      if (!btn) return;
      var act = btn.getAttribute('data-act');
      if (act === 'zoomin') zoomAt(BUTTON_STEP);
      else if (act === 'zoomout') zoomAt(1 / BUTTON_STEP);
      else if (act === 'reset') reset();
      else if (act === 'fullscreen') {
        if (!document.fullscreenElement) {
          if (figure.requestFullscreen) figure.requestFullscreen();
        } else {
          if (document.exitFullscreen) document.exitFullscreen();
        }
      }
    });

    // Re-apply after fullscreen exit (viewport changed)
    document.addEventListener('fullscreenchange', function () {
      if (document.fullscreenElement === figure || document.fullscreenElement === null) {
        clamp(); apply();
      }
    });

    reset();
  }

  function initAll() {
    var figs = document.querySelectorAll('figure.svg-figure');
    for (var i = 0; i < figs.length; i++) initViewer(figs[i]);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initAll);
  } else {
    initAll();
  }

  // Reveal.js may re-render on fragment navigation
  window.addEventListener('hashchange', function () { setTimeout(initAll, 150); });

  // Catch dynamically-inserted figures (shouldn't happen with static export but defensive)
  var mo = new MutationObserver(function (muts) {
    for (var i = 0; i < muts.length; i++) {
      var added = muts[i].addedNodes;
      for (var j = 0; j < added.length; j++) {
        var n = added[j];
        if (n.nodeType !== 1) continue;
        if (n.matches && n.matches('figure.svg-figure')) initViewer(n);
        if (n.querySelectorAll) {
          var inner = n.querySelectorAll('figure.svg-figure');
          for (var k = 0; k < inner.length; k++) initViewer(inner[k]);
        }
      }
    }
  });
  mo.observe(document.body, { childList: true, subtree: true });
})();
