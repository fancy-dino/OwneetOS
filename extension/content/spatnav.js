// SPDX-License-Identifier: GPL-3.0-or-later
//
// Spatial navigation for web pages ([[extension-design]], [[controller-input-map]]): which elements
// the D-pad can reach, and which one comes next in a direction. The geometry (choose) works on
// plain rectangles so that it can be tested without a browser (spatnav.test.js).

'use strict';

var OwneetNav = (() => {
  const NATIVE = [
    'a[href]', 'area[href]', 'button', 'input:not([type="hidden"])', 'select', 'textarea', 'summary',
    '[contenteditable=""]', '[contenteditable="true"]', '[contenteditable="plaintext-only"]', '[tabindex]',
  ];
  // Plain elements made clickable by the site's code, marked only with a role
  const ROLES = ['button', 'link', 'tab', 'menuitem', 'menuitemcheckbox', 'menuitemradio', 'option', 'checkbox',
    'radio', 'switch', 'gridcell', 'treeitem', 'slider', 'textbox', 'searchbox', 'combobox'];
  const SELECTOR = NATIVE.concat(ROLES.map(r => `[role="${r}"]`)).join(', ');
  const SLACK = 4;          // px of overlap still counted as "past" the current element

  // ---------- Geometry (no DOM) ----------

  // Is rectangle r past c in direction dir?
  function past(c, r, dir) {
    switch (dir) {
      case 'right': return r.left >= c.right - SLACK;
      case 'left': return r.right <= c.left + SLACK;
      case 'down': return r.top >= c.bottom - SLACK;
      default: return r.bottom <= c.top + SLACK;
    }
  }

  // Gap along the direction and offset across it (0 when the two overlap across).
  function measure(c, r, dir) {
    const horiz = dir === 'left' || dir === 'right';
    const gap = dir === 'right' ? r.left - c.right : dir === 'left' ? c.left - r.right
      : dir === 'down' ? r.top - c.bottom : c.top - r.bottom;
    const [a0, a1, b0, b1] = horiz ? [c.top, c.bottom, r.top, r.bottom] : [c.left, c.right, r.left, r.right];
    return { gap: Math.max(0, gap), across: Math.max(0, b0 - a1, a0 - b1) };
  }

  // The best candidate from origin rectangle c: candidates are {rect, sameRow}. Distance = gap +
  // 2 × offset across; an overlapping one wins a tie. Leaving the current scrolling row sideways
  // needs an overlap across, so the end of a row does not jump into another row.
  function choose(c, candidates, dir) {
    const horiz = dir === 'left' || dir === 'right';
    let best = -1, bestScore = Infinity;
    candidates.forEach((cand, i) => {
      const r = cand.rect;
      if (!past(c, r, dir)) return;
      const { gap, across } = measure(c, r, dir);
      if (horiz && across > 0 && !cand.sameRow) return;
      const score = gap + 2 * across - (across === 0 ? 0.5 : 0);
      if (score < bestScore) { bestScore = score; best = i; }
    });
    return best;
  }

  // Reading order: line by line (centres closer than 20 px share a line), then left to right.
  function readingOrder(a, b) {
    const ca = a.top + a.height / 2, cb = b.top + b.height / 2;
    return Math.abs(ca - cb) > 20 ? ca - cb : a.left - b.left;
  }

  // ---------- DOM ----------

  // The parent across shadow roots.
  const up = el => el.parentElement || (el.parentNode && el.parentNode.host) || null;

  // The element's rectangle in the top page's coordinates (adds the offsets of same-origin frames).
  function rectOf(el) {
    const r = el.getBoundingClientRect();
    let left = r.left, top = r.top;
    try {
      for (let w = el.ownerDocument.defaultView; w && w.frameElement; w = w.parent) {
        const f = w.frameElement.getBoundingClientRect();
        left += f.left + w.frameElement.clientLeft; top += f.top + w.frameElement.clientTop;
      }
    } catch (_) { /* a cross-origin frame: not searched anyway */ }
    return { left, top, right: left + r.width, bottom: top + r.height, width: r.width, height: r.height };
  }

  function usable(el) {
    if (!el.isConnected || el === el.ownerDocument.body || el === el.ownerDocument.documentElement) return false;
    if (el.disabled || el.getAttribute('aria-disabled') === 'true') return false;
    if (el.closest('[inert], [aria-hidden="true"]')) return false;
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) return false;
    if (el.checkVisibility && !el.checkVisibility({ visibilityProperty: true })) return false;
    return true;
  }

  // Every element matching the selector in root, its open shadow roots and same-origin frames.
  function collect(root, out) {
    root.querySelectorAll(SELECTOR).forEach(el => out.push(el));
    root.querySelectorAll('*').forEach(el => {
      if (el.shadowRoot) collect(el.shadowRoot, out);
      if (el.tagName === 'IFRAME' || el.tagName === 'FRAME') {
        let doc = null;
        try { doc = el.contentDocument; } catch (_) { /* cross-origin */ }
        if (doc && doc.documentElement) collect(doc, out);
      }
    });
    return out;
  }

  // The targets of the D-pad; a container that holds other targets loses to them (the smallest
  // wins). skip: an element of the extension's own (the ring).
  function targets(doc, skip) {
    const all = collect(doc, []).filter(el => el !== skip && el.tagName !== 'IFRAME' && el.tagName !== 'FRAME' && usable(el));
    const set = new Set(all), containers = new Set();
    for (const el of all) {
      for (let p = up(el); p; p = up(p)) {
        if (containers.has(p)) break;
        if (set.has(p)) containers.add(p);
      }
    }
    return all.filter(el => !containers.has(el));
  }

  // The element's nearest scrolling container (the page's scrolling element if none).
  function scroller(el) {
    for (let p = up(el); p; p = up(p)) {
      if (p === p.ownerDocument.body || p === p.ownerDocument.documentElement) break;
      const s = getComputedStyle(p);
      const x = /(auto|scroll)/.test(s.overflowX) && p.scrollWidth > p.clientWidth;
      const y = /(auto|scroll)/.test(s.overflowY) && p.scrollHeight > p.clientHeight;
      if (x || y) return p;
    }
    return (el.ownerDocument || document).scrollingElement || document.documentElement;
  }

  // The next target from origin (an element, or the rectangle where the last one was) in dir.
  function next(doc, origin, dir, skip) {
    const c = origin.getBoundingClientRect ? rectOf(origin) : origin;
    const row = origin.getBoundingClientRect ? scroller(origin) : null;
    const list = targets(doc, skip).filter(el => el !== origin && !(origin.contains && el.contains(origin)));
    const i = choose(c, list.map(el => ({ rect: rectOf(el), sameRow: row !== null && scroller(el) === row })), dir);
    return i < 0 ? null : list[i];
  }

  // The first target on screen in reading order.
  function first(doc, skip) {
    const h = innerHeight;
    const list = targets(doc, skip).map(el => ({ el, r: rectOf(el) }));
    const seen = list.filter(t => t.r.bottom > 0 && t.r.top < h);
    const pick = (seen.length ? seen : list).sort((a, b) => readingOrder(a.r, b.r))[0];
    return pick ? pick.el : null;
  }

  return { SELECTOR, choose, readingOrder, rectOf, usable, targets, scroller, next, first, up };
})();

if (typeof module !== 'undefined') module.exports = OwneetNav;
