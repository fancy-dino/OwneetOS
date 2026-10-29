// SPDX-License-Identifier: GPL-3.0-or-later
//
// Spotify's own navigation ([[step-5.5-spotify]]; [[extension-design]] "Spotify"; bindings:
// [[controller-input-map]] "Inside Spotify"). Spotify's desktop web player, driven with the ring of
// input.js (its player for phones and TVs keeps Your Library in Spotify's app, 2026-10-27).
//  - The D-pad moves by position only (owner, 2026-10-28 / 29): from the selection, the nearest
//    target past its edge in the pressed direction (spatialPick), wherever it is: the top bar, the
//    left sidebar (Your Library), the main view; inside the player bar, a menu or a window, the
//    same. No rows, columns or parts. A card, a song and a Your Library row are one target each
//    (their own buttons and links are not); Spotify's sliders are not stops. The page and a shelf
//    scroll to the selection. This geometry is Spotify's own: the generic one (spatnav.js) is not
//    used for moving here, and Spotify's changes never reach it.
//  - A plays a card, a Your Library row or a song at once (its own play button); elsewhere it
//    clicks (a scripted Enter does nothing on Spotify). Menu opens the selected item's page.
//  - B closes an open menu or Spotify window first (the selection stays inside them while they
//    are open), then the player bar, then the generic B: back a page, or "Close Spotify?".
//  - X: the selected item's context menu, then its "Add to queue" (found by its label: the menu's
//    items have no names of their own; in another language the menu stays open).
//  - Y: the selection to the player bar and back. LT / RT: Spotify's Shift+← / → (5 s). LB / RB:
//    nothing.
//  - On a TV: drawn as on a screen about 1280 px wide (layoutWidth: the worker sets Brave's zoom
//    for the tab), and Spotify's right panel folded away when the page opens and after A plays
//    (the desktop player reopens it when a song starts).
// "Install App", "Open in Desktop app" and "Open Miniplayer" are never reached: they open windows
// or programs the console cannot show.
//
// Spotify's names (data-testid, read with probes in the test VM on 2026-10-24) live in NAMES; what
// is not found falls back to the generic navigation. Tests: test/spotify.test.js; the pages
// themselves are checked in the test VM.

'use strict';

var OwneetSpotify = (() => {
  const { sendKey } = typeof OwneetShared !== 'undefined' ? OwneetShared : require('./shared.js');

  const NAMES = {
    top: '[data-testid="global-nav-bar"]',
    main: 'main',
    sidebar: 'nav',
    libraryRow: '[role="row"]',
    bar: '[data-testid="now-playing-bar"]',
    playPause: '[data-testid="control-button-playpause"]',
    shelf: '[data-testid="component-shelf"]',
    shelfArea: '[data-testid="carousel-scroller"]',
    shelfNext: '[data-testid="carousel-next-button"]',
    shelfPrev: '[data-testid="carousel-previous-button"]',
    card: '[data-encore-id="card"]',
    song: '[data-testid="tracklist-row"]',
    // a card's or Your Library row's own play button; a song's title link
    playButton: '[data-testid="play-button"]',
    songLink: '[data-testid="internal-track-link"]',
    // Spotify's sliders (progress, volume): not stops
    slider: 'input[type="range"], [role="slider"]',
    menu: '#context-menu, [data-testid="context-menu"]',
    menuItem: '[role="menuitem"]',
    window: '[role="dialog"], [aria-modal="true"]',
    // never reached: "Open Miniplayer" (a window of its own) and "Install App" (a download)
    never: '[data-testid="pip-toggle-button"], a[href*="/download"]',
  };
  // The labels Spotify gives the items found by their label, in the console's languages
  const QUEUE = [/^add to queue$/i, /^aggiungi (in|alla) coda$/i];
  const NEVER = [/desktop app$/i, /app desktop$/i, /^install(a)? (l'|the )?app$/i, /miniplayer/i];
  const LAYOUT_WIDTH = 1280;     // on a TV Spotify lays out as on a screen this wide (owner, 2026-10-24)
  const SLACK = 4;               // px of overlap still counted as past the selection's edge
  const SLIDE_MS = 450;          // a shelf sliding on with Spotify's arrow
  const MENU_MS = 350;           // a context menu appearing
  const PLAY_MS = 1500;          // a song starting (Spotify then reopens its right panel)
  const PANEL_GAP = 120;         // the main view ends this far from the right edge: the right panel is open

  const matches = loc => loc.hostname === 'open.spotify.com';
  const isQueue = label => QUEUE.some(r => r.test(label));
  const isNeverLabel = label => NEVER.some(r => r.test(label));

  // The D-pad, by position only (owner, 2026-10-28): from box c, the box to go to in dir, as an
  // index into rects (-1: none). Only boxes past c's edge in that direction count; among them, those
  // lined up with c across the direction first, then the smallest gap along the direction plus
  // twice the offset across it; ties: the first in the page's order. Boxes: {left, top, right, bottom}.
  function spatialPick(c, rects, dir) {
    const sideways = dir === 'left' || dir === 'right';
    let best = -1, bestLined = false, bestScore = Infinity;
    rects.forEach((r, i) => {
      const gap = dir === 'right' ? r.left - c.right : dir === 'left' ? c.left - r.right
        : dir === 'down' ? r.top - c.bottom : c.top - r.bottom;
      if (gap < -SLACK) return;
      const [a0, a1, b0, b1] = sideways ? [c.top, c.bottom, r.top, r.bottom] : [c.left, c.right, r.left, r.right];
      const across = Math.max(0, b0 - a1, a0 - b1);
      const lined = across === 0, score = Math.max(0, gap) + 2 * across;
      if (best < 0 || (lined && !bestLined) || (lined === bestLined && score < bestScore)) {
        best = i; bestLined = lined; bestScore = score;
      }
    });
    return best;
  }

  // env: { document, location, KeyboardEvent, MouseEvent, PointerEvent, setTimeout, now } of the
  // page; page: the ring of input.js { Nav, skip, current(), moveTo(el), click(el), openKeyboard(el) }
  function driver(env, page) {
    const doc = env.document, Nav = page.Nav;
    const q = (sel, root = doc) => root.querySelector(sel);
    const qa = (sel, root = doc) => Array.from(root.querySelectorAll(sel));
    const usable = el => !!el && Nav.usable(el);
    const label = el => (el.innerText || el.getAttribute('aria-label') || '').trim().replace(/\s+/g, ' ');
    // Spotify's own keys go to its main element, where the probe's keys worked
    const keyTarget = () => doc.getElementById('main') || doc.body;
    let barFrom = null;     // where the selection was before Y took it to the player bar
    let menuFrom = null;    // ... before a context menu or a Spotify window opened

    // ---- The parts
    const main = () => q(NAMES.main);
    const top = () => q(NAMES.top);
    const bar = () => q(NAMES.bar);
    const sidebar = () => qa(NAMES.sidebar).find(n => usable(n) && !n.closest(NAMES.main) && !n.closest(NAMES.top)) || null;
    // Spotify keeps closed windows in the page: an open one is on screen
    function onScreen(el) {
      const r = el.getBoundingClientRect(), view = doc.defaultView;
      return r.bottom > 0 && r.right > 0 && r.top < view.innerHeight && r.left < view.innerWidth;
    }
    const openMenu = () => qa(NAMES.menu).filter(m => usable(m) && onScreen(m)).pop() || null;
    const openWindow = () => qa(NAMES.window)
      .filter(w => usable(w) && onScreen(w) && !w.closest(NAMES.menu) && !q(NAMES.menu, w)).pop() || null;
    // The unit el belongs to (a card, a song, a Your Library row), or null; side: the sidebar
    function unitOf(el, side = sidebar()) {
      if (!el || !el.closest) return null;
      const u = el.closest(`${NAMES.card}, ${NAMES.song}`);
      if (u) return u;
      return side && side.contains(el) ? el.closest(NAMES.libraryRow) : null;
    }
    // The element of a unit that the ring selects
    const focusOf = u => u.matches(NAMES.song) ? u : (q('[role="button"]', u) || u);
    const toUnit = (el, side) => { const u = unitOf(el, side); return u ? focusOf(u) : el; };
    // a unit is never one of these (side: the sidebar); Spotify's sliders are not stops either
    function isNever(el, side) {
      if (el.closest(NAMES.never) || el.matches(NAMES.slider)) return true;
      return !unitOf(el, side) && el.matches('a, button, [role="menuitem"]') && isNeverLabel(label(el));
    }

    // The D-pad's targets: the top bar's, the sidebar's and the main view's (not the player bar's,
    // reached with Y, nor the right panel's); a unit counts once
    function targets() {
      const side = sidebar(), seen = new Set(), out = [];
      for (const root of [top(), side, main()]) {
        if (!usable(root)) continue;
        for (const el of Nav.targets(root, page.skip, true)) {
          const t = toUnit(el, side);
          if (seen.has(t) || !usable(t) || isNever(t, side)) continue;
          seen.add(t);
          out.push(t);
        }
      }
      return out;
    }
    // The selection, a shelf scrolled (or slid with Spotify's arrow) so that it shows; the page
    // scrolls to it in moveTo
    function go(el) {
      const shelf = el.closest(NAMES.shelf), area = shelf && q(NAMES.shelfArea, shelf);
      if (area) {
        const a = area.getBoundingClientRect(), r = el.getBoundingClientRect();
        const dx = r.left < a.left ? r.left - a.left - 16 : r.right > a.right ? r.right - a.right + 16 : 0;
        if (dx) {
          let s = el.parentElement;
          while (s && s !== shelf && s.scrollWidth <= s.clientWidth + 1) s = s.parentElement;
          const before = s && s !== shelf ? s.scrollLeft : null;
          if (before !== null) s.scrollBy({ left: dx });
          if (before === null || s.scrollLeft === before) {
            // a shelf that does not scroll: Spotify's arrow slides it on
            const arrow = q(dx > 0 ? NAMES.shelfNext : NAMES.shelfPrev, shelf);
            if (usable(arrow)) {
              page.click(arrow);
              env.setTimeout(() => page.moveTo(usable(el) ? el : toUnit(Nav.first(shelf, page.skip))), SLIDE_MS);
              return;
            }
          }
        }
      }
      page.moveTo(el);
    }
    function verticalScroller(el) {
      const view = doc.defaultView;
      for (let p = el && el.parentElement; p && p !== doc.body; p = p.parentElement) {
        const s = view.getComputedStyle(p);
        if (/(auto|scroll)/.test(s.overflowY) && p.scrollHeight > p.clientHeight) return p;
      }
      return doc.scrollingElement;
    }

    // ---- Moving in the page, by position only
    function browse(dir, again) {
      if (!usable(main())) return false;               // not the player's page
      const list = targets();
      const cur = toUnit(page.current());
      if (!cur || !list.includes(cur)) {
        const first = toUnit(Nav.first(main(), page.skip));
        if (first) page.moveTo(first);
        return true;
      }
      const els = list.filter(el => el !== cur && !el.contains(cur) && !cur.contains(el));
      const i = spatialPick(Nav.rectOf(cur), els.map(el => Nav.rectOf(el)), dir);
      if (i >= 0) { go(els[i]); return true; }
      // nothing in that direction: up / down scroll the page a step (Spotify draws long lists as
      // they come), then try once more
      if ((dir === 'up' || dir === 'down') && !again) {
        const s = verticalScroller(cur);
        s.scrollBy({ top: (dir === 'down' ? 0.4 : -0.4) * s.clientHeight });
        env.setTimeout(() => browse(dir, true), 300);
      }
      return true;
    }

    // ---- Menus, Spotify's windows, the player bar: the selection stays inside while they are open
    // remember: B brings the selection back where it was (a menu, a window)
    function inside(root, dir, remember) {
      const cur = page.current();
      if (!cur || !root.contains(cur)) {
        if (remember && !menuFrom) menuFrom = cur;
        enter(root);
        return true;
      }
      const els = Nav.targets(root, page.skip).filter(el => el !== cur && !el.contains(cur) && !isNever(el));
      const i = spatialPick(Nav.rectOf(cur), els.map(el => Nav.rectOf(el)), dir);
      if (i >= 0) page.moveTo(els[i]);
      return true;
    }
    function enter(root) {
      const first = Nav.targets(root, page.skip).find(el => !isNever(el));
      if (first) page.moveTo(first);
    }
    // The selected item's context menu (as a right click), then `then` once it is open
    function openMenuFor(el, then) {
      menuFrom = el;
      const r = el.getBoundingClientRect();
      el.dispatchEvent(new env.MouseEvent('contextmenu', { bubbles: true, cancelable: true, composed: true,
        view: doc.defaultView, button: 2, buttons: 2, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2 }));
      env.setTimeout(() => { const m = openMenu(); if (m) then(m); }, MENU_MS);
    }
    // The selection back where it was before a menu or a window
    function backFrom(ms) {
      const from = menuFrom;
      menuFrom = null;
      env.setTimeout(() => { if (usable(from)) page.moveTo(from); }, ms);
    }
    function leaveBar() {
      const from = barFrom;
      barFrom = null;
      if (usable(from)) { page.moveTo(from); return; }
      const m = main(), first = usable(m) ? toUnit(Nav.first(m, page.skip)) : null;
      if (first) page.moveTo(first);
    }
    const inBar = cur => { const b = bar(); return !!(cur && b && b.contains(cur)); };

    // ---- On a TV: Spotify's right panel folded away once when the page opens (its own Alt+Shift+R)
    function foldPanel(tries) {
      const m = main();
      if (!usable(m)) { if (tries > 0) env.setTimeout(() => foldPanel(tries - 1), 500); return; }
      if (m.getBoundingClientRect().right < doc.defaultView.innerWidth - PANEL_GAP) {
        sendKey(env, keyTarget(), 'R', 'KeyR', 82, { alt: true, shift: true });
      }
    }
    env.setTimeout(() => foldPanel(20), 1000);
    const afterPlay = () => env.setTimeout(() => foldPanel(0), PLAY_MS);

    return {
      layoutWidth: LAYOUT_WIDTH,
      move(dir) {
        const m = openMenu();
        if (m) return inside(m, dir, true);
        const w = openWindow();
        if (w) return inside(w, dir, true);
        menuFrom = null;                               // closed by itself (e.g. one of its items went to a page)
        const cur = page.current();
        if (inBar(cur)) return inside(bar(), dir, false);
        return browse(dir, false);
      },
      press() {
        const m = openMenu() || openWindow(), cur = page.current();
        if (m && !(cur && m.contains(cur))) {
          if (!menuFrom) menuFrom = cur;
          enter(m);
          return true;
        }
        const unit = unitOf(cur);
        if (unit) {
          // a card, a Your Library row or a song plays at once with its own play button (a click
          // on a song's row only selects it; a song's play button is its first one)
          const song = unit.matches(NAMES.song), play = song ? q('button', unit) : q(NAMES.playButton, unit);
          if (usable(play)) page.click(play);
          else if (song) {
            const r = unit.getBoundingClientRect();
            unit.dispatchEvent(new env.MouseEvent('dblclick', { bubbles: true, cancelable: true, composed: true,
              view: doc.defaultView, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2 }));
          } else return false;                         // no play button of its own: the generic A opens it
          afterPlay();
          return true;
        }
        return false;                                  // the generic A clicks it (a text field: the keyboard)
      },
      back() {
        if (doc.fullscreenElement) return false;       // the generic B leaves Spotify's full screen
        const m = openMenu();
        if (m) {
          sendKey(env, doc.activeElement || m, 'Escape', 'Escape', 27);
          backFrom(200);
          return true;
        }
        const w = openWindow();
        if (w) {
          sendKey(env, doc.activeElement || w, 'Escape', 'Escape', 27);
          backFrom(300);
          return true;
        }
        if (barFrom || inBar(page.current())) { leaveBar(); return true; }
        return false;                                  // the generic B: back a page, or "Close Spotify?"
      },
      // X: the selected item into the queue (its context menu's "Add to queue")
      secondary() {
        const unit = unitOf(page.current());
        if (!unit || openMenu() || openWindow()) return true;
        openMenuFor(focusOf(unit), m => {
          const item = qa(NAMES.menuItem, m).find(it => usable(it) && isQueue(label(it)));
          if (!item) { enter(m); return; }             // another language: the menu stays open
          page.click(item);
          backFrom(300);
        });
        return true;
      },
      // Menu: the selected item's page (a card or a Your Library row through its own link, a song
      // through its title)
      options() {
        const unit = unitOf(page.current());
        if (!unit || openMenu() || openWindow()) return true;
        const open = unit.matches(NAMES.song) ? (q(NAMES.songLink, unit) || q('a[href]', unit)) : focusOf(unit);
        if (usable(open)) page.click(open);
        return true;
      },
      // Y: the selection to the player bar and back
      page() {
        if (openMenu() || openWindow()) return true;
        const b = bar(), cur = page.current();
        if (!usable(b)) return true;
        if (inBar(cur)) { leaveBar(); return true; }
        barFrom = cur;
        const pp = q(NAMES.playPause, b);
        if (usable(pp)) page.moveTo(pp);
        else enter(b);
        return true;
      },
      // LT / RT: back / forward 5 s, Spotify's Shift+← / →
      seek(step) {
        sendKey(env, keyTarget(), step > 0 ? 'ArrowRight' : 'ArrowLeft', step > 0 ? 'ArrowRight' : 'ArrowLeft',
          step > 0 ? 39 : 37, { shift: true });
        return true;
      },
      // the right stick: up / down scroll the main view or the sidebar (not a shelf), sideways a shelf
      scroll(dx, dy) {
        const cur = page.current();
        const from = cur && Nav.usable(cur) ? cur : main();
        if (!from) return false;
        if (dy) verticalScroller(from).scrollBy({ top: dy });
        if (dx) Nav.scroller(from).scrollBy({ left: dx });
        return true;
      },
    };
  }

  return { NAMES, matches, isQueue, isNeverLabel, spatialPick, LAYOUT_WIDTH, driver };
})();

if (typeof module !== 'undefined') module.exports = OwneetSpotify;
