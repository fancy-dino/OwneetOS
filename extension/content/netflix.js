// SPDX-License-Identifier: GPL-3.0-or-later
//
// Netflix's own navigation ([[step-5.4-netflix]]; [[extension-design]] "Netflix"; bindings:
// [[controller-input-map]] "Inside Netflix"). Netflix has no TV interface on the web: its desktop
// site, driven part by part, with the ring of input.js.
//  - Browsing pages: a grid of rows. Up / down go from row to row keeping about the same column
//    (the banner and the top bar above the first row), left / right from tile to tile, the row
//    sliding on (Netflix's own arrow) at the edge of what is visible. A on a tile opens the title
//    window and the selection goes into it; B closes it and the selection goes back to the tile.
//  - Player: any button brings up Netflix's controls (as a mouse movement does) and the D-pad moves
//    between them; A presses one, and pauses / plays while they are hidden; LT / RT seek by 10 s
//    (Netflix's ← / →); B closes an open menu, otherwise leaves the player.
//  - Y opens Netflix's search, with the console's keyboard.
// Every other page (start, sign-in, the email code, profiles, search results…) keeps the generic
// navigation: the driver's methods answer false there.
//
// Netflix's element names (data-uia, read with probes in the test VM on 2026-10-22) live in NAMES:
// Netflix renames its site from time to time, and what is not found falls back to the generic
// navigation. The geometry is tested without a browser (test/netflix.test.js).

'use strict';

var OwneetNetflix = (() => {
  const NAMES = {
    row: '[data-uia^="carousel-row-section"]',
    rowArea: '[data-uia="carousel-scroller"]',
    rowNext: '[data-uia="carousel-hawkins-right-button"]',
    rowPrev: '[data-uia="carousel-hawkins-left-button"]',
    card: ['standard-card', 'episodic-card', 'progress-card', 'ranked-card', 'cloud-game-card']
      .map(n => `[data-uia="${n}"]`).join(', '),
    top: '[data-uia="navigation+subheader"]',
    banner: '[data-uia="billboard"]',
    bannerPlay: '[data-uia="play-video-button"]',
    search: '[data-uia="navigation+actions+search+icon"]',
    searchInput: '[data-uia="navigation+actions+search+input"]',
    title: '[data-uia^="modal-motion-container-DETAIL_MODAL"]',
    titlePlay: '[data-uia="play-button"]',
    titleClose: '[data-uia="previewModal-closebtn"]',
    player: '[data-uia="watch-video"]',
    playerKeys: '[data-uia="player"]',
    control: '[data-uia^="control-"], [data-uia="timeline-knob"]',
    playPause: '[data-uia^="control-play-pause"]',
    leave: '[data-uia="control-nav-back"]',
    menu: '[role="dialog"], [role="menu"], [role="listbox"], [data-uia*="selector"], [data-uia*="popup"]',
  };
  const CONTROLS_SHOWN_MS = 2800;   // Netflix hides its controls a few seconds after the last movement
  const SLIDE_MS = 450;             // a row sliding on
  const TITLE_OPEN_MS = 700;        // the title window appearing
  const TITLE_CLOSE_MS = 400;       // and closing

  const matches = loc => /(^|\.)netflix\.com$/.test(loc.hostname);

  // The index of the box whose centre is nearest to x across (-1: none)
  function nearestTo(x, rects) {
    let best = -1, bestD = Infinity;
    rects.forEach((r, i) => {
      const d = Math.abs(r.left + r.width / 2 - x);
      if (d < bestD) { best = i; bestD = d; }
    });
    return best;
  }
  // Is the box fully inside the area across (left / right; 2 px of slack)?
  const insideAcross = (r, area) => r.left >= area.left - 2 && r.right <= area.right + 2;
  // After a row slid on: the tile to select among the visible ones (rects), given the label of the
  // tile that was wanted (labels) and the direction (1: right, -1: left)
  function afterSlide(labels, wanted, step) {
    const same = wanted ? labels.indexOf(wanted) : -1;
    if (same >= 0) return same;
    return labels.length ? (step > 0 ? 0 : labels.length - 1) : -1;
  }

  // env: { document, location, KeyboardEvent, MouseEvent, setTimeout, now } of the page; page: the
  // ring of input.js { Nav, skip, current(), moveTo(el), click(el), openKeyboard(el) }
  function driver(env, page) {
    const doc = env.document, loc = env.location, Nav = page.Nav;
    const q = (sel, root = doc) => root.querySelector(sel);
    const qa = (sel, root = doc) => Array.from(root.querySelectorAll(sel));
    const usable = el => !!el && Nav.usable(el);
    const centreX = el => { const r = Nav.rectOf(el); return r.left + r.width / 2; };
    let column = null;            // the x kept while moving up / down between rows
    let titleFrom = null;         // what opened the title window (the selection goes back there)
    let controlsAt = -Infinity;   // when the player's controls were last brought up
    let wiggle = 1;               // a mouse movement must move to count

    function sendKey(target, key, code, keyCode) {
      for (const type of ['keydown', 'keyup']) {
        target.dispatchEvent(new env.KeyboardEvent(type, { key, code, keyCode, which: keyCode,
          bubbles: true, cancelable: true, composed: true }));
      }
    }

    // ---- What is on screen
    const player = () => loc.pathname.startsWith('/watch/') ? q(NAMES.player) : null;
    function titleWindow() { const t = q(NAMES.title); return usable(t) ? t : null; }
    const rows = () => qa(NAMES.row).filter(usable);
    // The tiles of a row, outermost only, in the page's order
    const cardsIn = row => qa(NAMES.card, row).filter(el => usable(el) && !el.parentElement.closest(NAMES.card));
    const cardOf = el => el && el.closest(NAMES.card);
    function partOf(el) {          // the part of the browsing page el is in
      if (!el) return null;
      for (const [kind, sel] of [['row', NAMES.row], ['banner', NAMES.banner], ['top', NAMES.top]]) {
        const root = el.closest(sel);
        if (root) return { kind, root };
      }
      return null;
    }
    function rowArea(row) {
      const r = (q(NAMES.rowArea, row) || row).getBoundingClientRect();
      return { left: r.left, right: r.right };
    }
    const visibleIn = (row, cards) => { const area = rowArea(row); return cards.filter(c => insideAcross(Nav.rectOf(c), area)); };

    // ---- Browsing pages
    function bannerTarget() {
      const banner = q(NAMES.banner);
      if (!usable(banner)) return null;
      const play = q(NAMES.bannerPlay, banner);
      return usable(play) ? play : Nav.first(banner, page.skip);
    }
    function toTop() {
      const top = q(NAMES.top);
      const el = usable(top) ? Nav.first(top, page.skip) : null;
      if (el) page.moveTo(el);
      return true;
    }
    function within(root, cur, dir) {
      const el = Nav.next(root, cur, dir, page.skip);
      if (el) page.moveTo(el);
      return true;
    }
    function toRow(row, cur) {
      if (column === null) column = cur ? centreX(cur) : 0;
      const cards = cardsIn(row), shown = visibleIn(row, cards), pool = shown.length ? shown : cards;
      const j = nearestTo(column, pool.map(c => Nav.rectOf(c)));
      if (j >= 0) page.moveTo(pool[j]);              // the page scrolls to it
      return true;
    }
    function slide(row, cur, step) {
      const cards = cardsIn(row), next = cards[cards.indexOf(cardOf(cur)) + step];
      column = null;
      if (next && insideAcross(Nav.rectOf(next), rowArea(row))) {
        page.moveTo(next);
        return true;
      }
      // the edge of what is visible: Netflix's arrow slides the row on, then the next tile
      const arrow = q(step > 0 ? NAMES.rowNext : NAMES.rowPrev, row);
      if (!usable(arrow)) {
        if (next) page.moveTo(next);
        return true;
      }
      const wanted = next ? next.getAttribute('aria-label') : null;
      page.click(arrow);
      env.setTimeout(() => {
        const shown = visibleIn(row, cardsIn(row));
        const i = afterSlide(shown.map(c => c.getAttribute('aria-label')), wanted, step);
        if (i >= 0) page.moveTo(shown[i]);
      }, SLIDE_MS);
      return true;
    }
    function browse(dir) {
      const list = rows();
      if (!list.length) return false;                // not a browsing page
      const cur = page.current(), at = partOf(cur);
      if (!at) {                                     // nothing selected yet
        const start = bannerTarget() || cardsIn(list[0])[0];
        if (start) page.moveTo(start);
        return true;
      }
      const sideways = dir === 'left' || dir === 'right';
      if (at.kind === 'row') {
        if (sideways) return slide(at.root, cur, dir === 'right' ? 1 : -1);
        const target = list[list.indexOf(at.root) + (dir === 'down' ? 1 : -1)];
        if (target) return toRow(target, cur);
        if (dir === 'up') {
          const b = bannerTarget();
          if (b) { page.moveTo(b); return true; }
          return toTop();
        }
        // below the last row: Netflix adds rows as the page scrolls
        doc.scrollingElement.scrollBy({ top: 0.6 * doc.defaultView.innerHeight });
        return true;
      }
      if (at.kind === 'banner') {
        if (dir === 'down') return toRow(list[0], cur);
        if (dir === 'up') return toTop();
        return within(at.root, cur, dir);
      }
      // the top bar
      if (dir === 'down') {
        const b = bannerTarget();
        if (b) { page.moveTo(b); return true; }
        return toRow(list[0], cur);
      }
      return within(at.root, cur, dir);
    }

    // ---- The title window
    function enterTitle(t) {
      const play = q(NAMES.titlePlay, t);
      const el = usable(play) ? play : Nav.first(t, page.skip);
      if (el) page.moveTo(el);
    }
    function inTitle(t, dir) {
      const cur = page.current();
      if (!cur || !t.contains(cur)) { enterTitle(t); return true; }
      return within(t, cur, dir);
    }

    // ---- The player
    function showControls(pl) {
      const r = pl.getBoundingClientRect();
      wiggle = -wiggle;
      pl.dispatchEvent(new env.MouseEvent('mousemove', { bubbles: true,
        clientX: r.left + r.width / 2 + 8 * wiggle, clientY: r.top + r.height / 2 }));
      controlsAt = env.now();
    }
    const controlsShown = () => env.now() - controlsAt < CONTROLS_SHOWN_MS;
    const keysTarget = pl => q(NAMES.playerKeys, pl) || pl;
    function playerMove(pl, dir) {
      const shown = controlsShown(), cur = page.current();
      showControls(pl);
      // just brought up (or the selection on the whole player): the selection on play / pause
      if (!shown || !cur || !pl.contains(cur) || cur === keysTarget(pl)) {
        const pp = q(NAMES.playPause, pl);
        if (usable(pp)) page.moveTo(pp);
        return true;
      }
      return within(pl, cur, dir);
    }
    function playerPress(pl) {
      const shown = controlsShown(), cur = page.current();
      showControls(pl);
      if (shown && cur && pl.contains(cur) && cur !== keysTarget(pl)) return false;   // presses it
      sendKey(keysTarget(pl), ' ', 'Space', 32);      // controls hidden: pause / play
      return true;
    }
    function playerBack(pl) {
      const menu = qa(NAMES.menu, pl).find(el => usable(el) && !el.matches(NAMES.control));
      if (menu) {
        sendKey(doc.activeElement || pl, 'Escape', 'Escape', 27);
        return true;
      }
      const leave = q(NAMES.leave, pl);
      if (!usable(leave)) return false;              // the generic B goes back a page
      page.click(leave);
      return true;
    }

    return {
      move(dir) {
        const pl = player();
        if (pl) return playerMove(pl, dir);
        const t = titleWindow();
        if (t) return inTitle(t, dir);
        return browse(dir);
      },
      press() {
        const pl = player();
        if (pl) return playerPress(pl);
        const t = titleWindow(), cur = page.current();
        if (t) {
          if (cur && t.contains(cur)) return false;  // the generic A presses it
          enterTitle(t);
          return true;
        }
        if (partOf(cur) && rows().length) {
          // a tile or the banner's "More info": its title window opens, the selection goes in
          titleFrom = cur;
          env.setTimeout(() => { const w = titleWindow(); if (w) enterTitle(w); }, TITLE_OPEN_MS);
        }
        return false;                                 // the generic A presses it
      },
      back() {
        const pl = player();
        if (pl) return playerBack(pl);
        const t = titleWindow();
        if (!t) return false;                         // the generic B: back a page, or "Close Netflix?"
        const close = q(NAMES.titleClose, t);
        if (usable(close)) page.click(close);
        else sendKey(doc.activeElement || doc.body, 'Escape', 'Escape', 27);
        const from = titleFrom;
        titleFrom = null;
        env.setTimeout(() => { if (usable(from)) page.moveTo(from); }, TITLE_CLOSE_MS);
        return true;
      },
      // LT / RT: back / forward 10 s in the player, nothing elsewhere
      seek(step) {
        const pl = player();
        if (pl) {
          showControls(pl);
          sendKey(keysTarget(pl), step > 0 ? 'ArrowRight' : 'ArrowLeft', step > 0 ? 'ArrowRight' : 'ArrowLeft', step > 0 ? 39 : 37);
        }
        return true;
      },
      // Y: Netflix's search, with the console's keyboard
      page() {
        if (player() || titleWindow()) return true;
        const icon = q(NAMES.search);
        if (!usable(icon)) return true;
        page.click(icon);
        env.setTimeout(() => {
          const input = q(NAMES.searchInput);
          if (usable(input)) page.openKeyboard(input);
        }, 300);
        return true;
      },
      // the right stick: nothing in the player, the generic scrolling elsewhere
      scroll: () => !!player(),
      // the ring is hidden while the player's controls are
      hideRing: () => !!player() && !controlsShown(),
    };
  }

  return { NAMES, matches, nearestTo, insideAcross, afterSlide, driver };
})();

if (typeof module !== 'undefined') module.exports = OwneetNetflix;
