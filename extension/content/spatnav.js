// SPDX-License-Identifier: GPL-3.0-or-later
//
// Spatial navigation for web pages ([[extension-design]], [[controller-input-map]]): which elements
// the D-pad can reach, and which one comes next in a direction.
//
// How the next one is chosen (agreed with the owner on 2026-10-16, after comparing versions in the
// demo of step 5.2):
//  1. Structure first: the candidates are grouped by the smallest box (DOM ancestor) they share
//     with the current element; the search starts in the current element's own box and goes
//     outward only when nothing fits there.
//  2. In line first: a pass that only accepts candidates overlapping the current element across the
//     direction runs through every level before a second pass accepts diagonal ones.
//  3. Inside a group, the distance of the W3C draft "CSS Spatial Navigation Level 1"
//     (https://www.w3.org/TR/css-nav-1/, 2019): euclidean + displacement - alignment - sqrt(overlap).
//  4. The column (or row) is remembered: going down and back up returns to the same place.
//  5. Left / right stay in the container (the smallest box holding the current element and another
//     target); with nothing there, they go to the target just before / after the current element in
//     the page's order, wherever it is (the owner's rule).
//
// The geometry (pick, pickSideways) works on plain rectangles so that it can be tested without a browser
// (test/spatnav.test.js).

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

  const horizontal = dir => dir === 'left' || dir === 'right';

  // Is rectangle r past c in direction dir?
  function past(c, r, dir) {
    switch (dir) {
      case 'right': return r.left >= c.right - SLACK;
      case 'left': return r.right <= c.left + SLACK;
      case 'down': return r.top >= c.bottom - SLACK;
      default: return r.bottom <= c.top + SLACK;
    }
  }

  // Gap along the direction, offset across it (0 when the projections overlap) and the length of
  // that overlap.
  function measure(c, r, dir) {
    const gap = dir === 'right' ? r.left - c.right : dir === 'left' ? c.left - r.right
      : dir === 'down' ? r.top - c.bottom : c.top - r.bottom;
    const [a0, a1, b0, b1] = horizontal(dir) ? [c.top, c.bottom, r.top, r.bottom] : [c.left, c.right, r.left, r.right];
    return { along: Math.max(0, gap), across: Math.max(0, b0 - a1, a0 - b1), overlap: Math.max(0, Math.min(a1, b1) - Math.max(a0, b0)) };
  }

  // The W3C draft's distance: euclidean + displacement - alignment - sqrt(overlap area).
  function distance(c, r, dir) {
    const h = horizontal(dir);
    const { along, across, overlap } = measure(c, r, dir);
    const euclidean = Math.hypot(along, across);
    const size = h ? c.height : c.width;
    const displacement = (across + size / 2) * (h ? 30 : 2);
    const alignment = size > 0 ? (overlap / size) * 5 : 0;
    const w = Math.min(c.right, r.right) - Math.max(c.left, r.left);
    const v = Math.min(c.bottom, r.bottom) - Math.max(c.top, r.top);
    return euclidean + displacement - alignment - (w > 0 && v > 0 ? Math.sqrt(w * v) : 0);
  }

  // The remembered column (moving up / down) or row (left / right) replaces the current element's
  // position across the direction. anchor: {axis: 'v' | 'h', at, size}.
  function anchored(c, dir, anchor) {
    if (!anchor || anchor.axis !== (horizontal(dir) ? 'h' : 'v')) return c;
    return horizontal(dir)
      ? { ...c, top: anchor.at - anchor.size / 2, bottom: anchor.at + anchor.size / 2, height: anchor.size }
      : { ...c, left: anchor.at - anchor.size / 2, right: anchor.at + anchor.size / 2, width: anchor.size };
  }

  // The best candidate from rectangle c in direction dir, or -1. Candidates: {rect, level, sameRow}:
  // level 0 shares the smallest box with the current element, higher levels larger ones; sameRow:
  // in the same scrolling container (leaving one sideways needs an element in line).
  function pick(c, candidates, dir, anchor) {
    const o = anchored(c, dir, anchor);
    const ahead = [];
    candidates.forEach((cand, i) => {
      if (!past(c, cand.rect, dir)) return;
      const inLine = measure(o, cand.rect, dir).across === 0;
      if (!inLine && horizontal(dir) && !cand.sameRow) return;
      ahead.push({ i, level: cand.level, inLine, d: distance(o, cand.rect, dir) });
    });
    const levels = [...new Set(ahead.map(a => a.level))].sort((a, b) => a - b);
    for (const inLine of [true, false]) {
      for (const level of levels) {
        let best = null;
        for (const a of ahead) {
          if (a.level === level && a.inLine === inLine && (!best || a.d < best.d)) best = a;   // ties: page order
        }
        if (best) return best.i;
      }
    }
    return -1;
  }

  // Left / right (the owner's rule): rects in page order, from the current one's index, s..e the
  // range of its container. The best in the container (pick), else the target just before / after
  // the current one in the page's order (for the first / last of a row: the target before / after
  // the row; in a vertical list: the item above / below). Returns an index or -1.
  function pickSideways(c, rects, from, dir, s, e, anchor) {
    const inBox = [];
    for (let i = s; i <= e; i++) if (i !== from) inBox.push(i);
    const k = pick(c, inBox.map(i => ({ rect: rects[i], level: 0, sameRow: true })), dir, anchor);
    if (k >= 0) return inBox[k];
    if (dir === 'left') return from - 1;
    return from + 1 < rects.length ? from + 1 : -1;
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

  // Every element matching the selector in page order, with the contents of open shadow roots and
  // same-origin frames where their host element is.
  function collectInOrder(root, out) {
    for (let el = root.firstElementChild; el; el = el.nextElementSibling) {
      if (el.matches(SELECTOR)) out.push(el);
      if (el.shadowRoot) collectInOrder(el.shadowRoot, out);
      if (el.tagName === 'IFRAME' || el.tagName === 'FRAME') {
        let doc = null;
        try { doc = el.contentDocument; } catch (_) { /* cross-origin */ }
        if (doc && doc.documentElement) collectInOrder(doc, out);
      }
      collectInOrder(el, out);
    }
    return out;
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

  // The targets of the D-pad inside root (a document or an element), in page order when ordered; a
  // container that holds other targets loses to them (the smallest wins). skip: an element of the
  // extension's own (the ring).
  function targets(root, skip, ordered) {
    const all = (ordered ? collectInOrder(root, []) : collect(root, [])).filter(el => el !== skip && el.tagName !== 'IFRAME' && el.tagName !== 'FRAME' && usable(el));
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

  // The level of each element: how many boxes up from the current element the smallest box they
  // share is (0: the current element's parent). Elements sharing none (another frame) come last.
  function levelsFrom(origin) {
    const chain = new Map();
    let n = 0;
    for (let p = up(origin); p; p = up(p)) chain.set(p, n++);
    return el => {
      for (let p = up(el); p; p = up(p)) if (chain.has(p)) return chain.get(p);
      return n;
    };
  }

  // The next target from origin (an element, or the rectangle where the last one was) in dir, inside
  // root. anchor: the remembered column or row (see anchored).
  function next(root, origin, dir, skip, anchor) {
    const isEl = !!origin.getBoundingClientRect;
    const c = isEl ? rectOf(origin) : origin;
    if (isEl && horizontal(dir)) {
      const all = targets(root, skip, true), from = all.indexOf(origin);
      if (from >= 0) {
        const [s, e] = group(all, from);
        const i = pickSideways(c, all.map(rectOf), from, dir, s, e, anchor);
        return i < 0 ? null : all[i];
      }
    }
    const row = isEl ? scroller(origin) : null;
    const level = isEl ? levelsFrom(origin) : () => 0;
    const list = targets(root, skip, true).filter(el => el !== origin && !(isEl && el.contains(origin)));
    const i = pick(c, list.map(el => ({ rect: rectOf(el), level: level(el), sameRow: row !== null && scroller(el) === row })), dir, anchor);
    return i < 0 ? null : list[i];
  }

  // Does box contain el, across shadow roots?
  function within(box, el) {
    for (let p = el; p; p = up(p)) if (p === box) return true;
    return false;
  }

  // The range of list (in page order) inside the smallest box that holds list[from] and another
  // target. Its targets are consecutive, so a neighbour in the list tells whether a box holds one.
  function group(list, from) {
    for (let box = up(list[from]); box; box = up(box)) {
      const before = from > 0 && within(box, list[from - 1]);
      const after = from < list.length - 1 && within(box, list[from + 1]);
      if (!before && !after) continue;
      let s = from, e = from;
      while (s > 0 && within(box, list[s - 1])) s--;
      while (e < list.length - 1 && within(box, list[e + 1])) e++;
      return [s, e];
    }
    return [from, from];
  }


  // The first target on screen in reading order.
  function first(root, skip) {
    const view = root.getBoundingClientRect ? root.getBoundingClientRect() : { top: 0, bottom: innerHeight };
    const list = targets(root, skip).map(el => ({ el, r: rectOf(el) }));
    const seen = list.filter(t => t.r.bottom > view.top && t.r.top < view.bottom);
    const pick1 = (seen.length ? seen : list).sort((a, b) => readingOrder(a.r, b.r))[0];
    return pick1 ? pick1.el : null;
  }

  // The anchor after a move from rectangle c in dir: kept while moving on the same axis.
  function anchorAfter(anchor, c, dir) {
    const axis = horizontal(dir) ? 'h' : 'v';
    if (anchor && anchor.axis === axis) return anchor;
    return axis === 'v' ? { axis, at: c.left + c.width / 2, size: c.width } : { axis, at: c.top + c.height / 2, size: c.height };
  }

  return { SELECTOR, pick, pickSideways, distance, anchored, anchorAfter, readingOrder, rectOf, usable, targets, scroller, next, first, up };
})();

if (typeof module !== 'undefined') module.exports = OwneetNav;
