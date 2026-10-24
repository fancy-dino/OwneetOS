// SPDX-License-Identifier: GPL-3.0-or-later
//
// Spotify's own navigation ([[step-5.5-spotify]]; [[extension-design]] "Spotify"; bindings:
// [[controller-input-map]] "Inside Spotify"). Spotify has no TV interface on the web (a TV user
// agent gets its page for phones, which plays nothing): its desktop web player, driven part by part
// with the ring of input.js. Spotify's own arrow keys move only inside a song list, a menu or one
// shelf, never between them, so the D-pad is the extension's:
//  - The targets are the top bar's, the left sidebar's (Your Library) and the main view's; a card, a
//    song and a Your Library row are one target each (their own buttons and links are not). In a
//    shelf left / right go from card to card, sliding the shelf with Spotify's own arrows at the
//    edge of what is visible (right at its end: its "Show all"); up / down from a card go to the
//    next shelf's cards, about the same column. Elsewhere the nearest target in the direction.
//  - A clicks (a scripted Enter does nothing on Spotify); on a song it presses the song's play
//    button. B closes an open menu or Spotify window first (the selection stays inside them while
//    they are open), then the generic B: back a page, or "Close Spotify?".
//  - X: the selected item's context menu, then its "Add to queue" (found by its label: the menu's
//    items have no names of their own; in another language the menu stays open). Menu: the menu.
//  - Y: the selection to the player bar and back. LT / RT: Spotify's Shift+← / → (5 s).
//  - LB / RB: Home → Search → Your Library, going round.
//  - On a TV: drawn as on a screen about 1280 px wide (layoutWidth: the worker sets Brave's zoom
//    for the tab), and Spotify's right panel folded away once when the page opens.
// "Install App", "Open in Desktop app" and "Open Miniplayer" are never reached: they open windows
// or programs the console cannot show.
//
// Spotify's names (data-testid, read with probes in the test VM on 2026-10-24) live in NAMES; what
// is not found falls back to the generic navigation. Tests: test/spotify.test.js; the pages
// themselves are checked in the test VM.

'use strict';

var OwneetSpotify = (() => {
  const { nearestTo, insideAcross, afterSlide, sendKey } = typeof OwneetShared !== 'undefined'
    ? OwneetShared : require('./shared.js');

  const NAMES = {
    top: '[data-testid="global-nav-bar"]',
    searchInput: '[data-testid="search-input"]',
    main: 'main',
    sidebar: 'nav',
    libraryRow: '[role="row"]',
    bar: '[data-testid="now-playing-bar"]',
    playPause: '[data-testid="control-button-playpause"]',
    shelf: '[data-testid="component-shelf"]',
    shelfArea: '[data-testid="carousel-scroller"]',
    shelfNext: '[data-testid="carousel-next-button"]',
    shelfPrev: '[data-testid="carousel-previous-button"]',
    showAll: '[data-testid="see-all-link"]',
    card: '[data-encore-id="card"]',
    cardTitle: '[data-encore-id="cardTitle"]',
    song: '[data-testid="tracklist-row"]',
    menu: '#context-menu, [data-testid="context-menu"]',
    menuItem: '[role="menuitem"]',
    window: '[role="dialog"], [aria-modal="true"]',
    // never reached: "Open Miniplayer" (a window of its own) and "Install App" (a download)
    never: '[data-testid="pip-toggle-button"], a[href*="/download"]',
  };
  // The labels Spotify gives the items found by their label, in the console's languages
  const QUEUE = [/^add to queue$/i, /^aggiungi (in|alla) coda$/i];
  const NEVER = [/desktop app$/i, /app desktop$/i, /^install(a)? (l'|the )?app$/i, /miniplayer/i];
  const SECTIONS = ['home', 'search', 'library'];
  const LAYOUT_WIDTH = 1280;     // on a TV Spotify lays out as on a screen this wide (owner, 2026-10-24)
  const SLIDE_MS = 450;          // a shelf sliding on
  const MENU_MS = 350;           // a context menu appearing
  const SECTION_MS = 700;        // a section's page appearing
  const PANEL_GAP = 120;         // the main view ends this far from the right edge: the right panel is open

  const matches = loc => loc.hostname === 'open.spotify.com';
  const isQueue = label => QUEUE.some(r => r.test(label));
  const isNeverLabel = label => NEVER.some(r => r.test(label));
  // LB / RB: the section after now (step 1) or before it (-1), going round
  const sectionAfter = (now, step) => SECTIONS[(SECTIONS.indexOf(now) + step + SECTIONS.length) % SECTIONS.length];

  // env: { document, location, KeyboardEvent, MouseEvent, PointerEvent, setTimeout, now } of the
  // page; page: the ring of input.js { Nav, skip, current(), moveTo(el), click(el), openKeyboard(el) }
  function driver(env, page) {
    const doc = env.document, loc = env.location, Nav = page.Nav;
    const q = (sel, root = doc) => root.querySelector(sel);
    const qa = (sel, root = doc) => Array.from(root.querySelectorAll(sel));
    const usable = el => !!el && Nav.usable(el);
    const label = el => (el.innerText || el.getAttribute('aria-label') || '').trim().replace(/\s+/g, ' ');
    const centreX = el => { const r = Nav.rectOf(el); return r.left + r.width / 2; };
    // Spotify's own keys go to its main element, where the probe's keys worked
    const keyTarget = () => doc.getElementById('main') || doc.body;
    let column = null;      // the x kept while moving up / down between shelves
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
    // a unit is never one of these (side: the sidebar)
    function isNever(el, side) {
      if (el.closest(NAMES.never)) return true;
      return !unitOf(el, side) && el.matches('a, button, [role="menuitem"]') && isNeverLabel(label(el));
    }

    // The D-pad's targets: the top bar's, the sidebar's and the main view's (not the player bar's,
    // reached with Y, nor the right panel's); a unit counts once
    function targets(roots) {
      const seen = new Set(), out = [];
      for (const root of [roots.top, roots.side, roots.main]) {
        if (!usable(root)) continue;
        for (const el of Nav.targets(root, page.skip, true)) {
          const t = toUnit(el, roots.side);
          if (seen.has(t) || !usable(t) || isNever(t, roots.side)) continue;
          seen.add(t);
          out.push(t);
        }
      }
      return out;
    }
    const regionOf = (roots, el) => roots.top && roots.top.contains(el) ? 'top'
      : roots.side && roots.side.contains(el) ? 'side' : roots.main && roots.main.contains(el) ? 'main' : null;
    // The nearest target in dir, staying in the current part when one fits as well (leaving
    // without: the parts nearby); outside: an element whose targets are not candidates
    function nearest(roots, cur, dir, list, outside) {
      const here = regionOf(roots, cur), els = [], cands = [];
      for (const el of list) {
        if (el === cur || el.contains(cur) || cur.contains(el) || (outside && outside.contains(el))) continue;
        els.push(el);
        cands.push({ rect: Nav.rectOf(el), level: regionOf(roots, el) === here ? 0 : 1, sameRow: true });
      }
      const i = Nav.pick(Nav.rectOf(cur), cands, dir, null);
      return i < 0 ? null : els[i];
    }

    // ---- Shelves
    const shelfOf = el => (el && el.closest && el.closest(NAMES.shelf)) || null;
    const cardsIn = shelf => qa(NAMES.card, shelf).filter(usable).map(focusOf);
    const cardLabel = el => { const t = q(NAMES.cardTitle, unitOf(el) || el); return (t || el).textContent.trim(); };
    function shelfArea(shelf) {
      const r = (q(NAMES.shelfArea, shelf) || shelf).getBoundingClientRect();
      return { left: r.left, right: r.right };
    }
    const visibleIn = (shelf, cards) => { const area = shelfArea(shelf); return cards.filter(c => insideAcross(Nav.rectOf(c), area)); };
    function toShelf(shelf, cur) {
      if (column === null) column = centreX(cur);
      const cards = cardsIn(shelf), shown = visibleIn(shelf, cards), pool = shown.length ? shown : cards;
      const j = nearestTo(column, pool.map(c => Nav.rectOf(c)));
      if (j >= 0) page.moveTo(pool[j]);
      return j >= 0;
    }
    // Left / right in a shelf; false: nothing more there (left at its start: the general move)
    function slide(shelf, cur, step) {
      const cards = cardsIn(shelf), next = cards[cards.indexOf(cur) + step];
      if (next && insideAcross(Nav.rectOf(next), shelfArea(shelf))) { page.moveTo(next); return true; }
      const arrow = q(step > 0 ? NAMES.shelfNext : NAMES.shelfPrev, shelf);
      if (next && usable(arrow)) {
        // the edge of what is visible: Spotify's arrow slides the shelf on, then the next card
        const wanted = cardLabel(next);
        page.click(arrow);
        env.setTimeout(() => {
          const shown = visibleIn(shelf, cardsIn(shelf));
          const i = afterSlide(shown.map(cardLabel), wanted, step);
          if (i >= 0) page.moveTo(shown[i]);
        }, SLIDE_MS);
        return true;
      }
      if (next) { page.moveTo(next); return true; }     // the page scrolls it into view
      if (step > 0) {
        const all = q(NAMES.showAll, shelf);
        if (usable(all)) page.moveTo(all);
        return true;
      }
      return false;
    }

    // ---- Moving in the page
    function browse(dir, again) {
      const roots = { top: top(), side: sidebar(), main: main() };
      if (!usable(roots.main)) return false;           // not the player's page
      const list = targets(roots);
      const cur = toUnit(page.current());
      if (!cur) {
        const first = toUnit(Nav.first(roots.main, page.skip));
        if (first) page.moveTo(first);
        return true;
      }
      if (!regionOf(roots, cur)) return false;         // a part this file does not know
      const sideways = dir === 'left' || dir === 'right';
      const unit = unitOf(cur), shelf = unit && unit.matches(NAMES.card) ? shelfOf(cur) : null;
      if (sideways) column = null;
      if (shelf && sideways && slide(shelf, cur, dir === 'right' ? 1 : -1)) return true;
      // up / down from a card skip its own shelf's title and "Show all"
      const el = nearest(roots, cur, dir, list, shelf && !sideways ? shelf : null);
      if (!el) {
        // nothing more: the page scrolls (Spotify draws long song lists as they come), then once more
        if (!sideways && !again) {
          const s = verticalScroller(cur);
          s.scrollBy({ top: (dir === 'down' ? 0.4 : -0.4) * s.clientHeight });
          env.setTimeout(() => browse(dir, true), 300);
        }
        return true;
      }
      const other = shelfOf(el);
      if (!sideways && other && other !== shelf && toShelf(other, cur)) return true;
      column = null;
      page.moveTo(el);
      return true;
    }
    function verticalScroller(el) {
      const view = doc.defaultView;
      for (let p = el && el.parentElement; p && p !== doc.body; p = p.parentElement) {
        const s = view.getComputedStyle(p);
        if (/(auto|scroll)/.test(s.overflowY) && p.scrollHeight > p.clientHeight) return p;
      }
      return doc.scrollingElement;
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
      let el = Nav.next(root, cur, dir, page.skip);
      while (el && isNever(el)) el = Nav.next(root, el, dir, page.skip);
      if (el) page.moveTo(el);
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
        if (unit && unit.matches(NAMES.song)) {
          // a song plays with its own play button (a click on the row only selects it)
          const play = q('button', unit);
          if (usable(play)) page.click(play);
          else {
            const r = unit.getBoundingClientRect();
            unit.dispatchEvent(new env.MouseEvent('dblclick', { bubbles: true, cancelable: true, composed: true,
              view: doc.defaultView, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2 }));
          }
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
        const cur = page.current(), unit = unitOf(cur);
        if (!unit || openMenu() || openWindow()) return true;
        openMenuFor(focusOf(unit), m => {
          const item = qa(NAMES.menuItem, m).find(it => usable(it) && isQueue(label(it)));
          if (!item) { enter(m); return; }             // another language: the menu stays open
          page.click(item);
          backFrom(300);
        });
        return true;
      },
      // Menu: the selected item's context menu, the selection inside it
      options() {
        const cur = page.current();
        if (!cur || openMenu() || openWindow()) return true;
        openMenuFor(toUnit(cur), enter);
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
      // LB / RB: Home → Search → Your Library, going round
      section(step) {
        if (openMenu() || openWindow()) return true;
        const side = sidebar(), cur = page.current();
        const now = cur && side && side.contains(cur) ? 'library' : loc.pathname.startsWith('/search') ? 'search' : 'home';
        const next = sectionAfter(now, step);
        barFrom = null;
        column = null;
        if (next === 'library') {
          const row = side ? qa(NAMES.libraryRow, side).filter(usable).map(focusOf)[0] : null;
          if (row) page.moveTo(row);
          return true;
        }
        if (next === 'home') sendKey(env, keyTarget(), 'H', 'KeyH', 72, { alt: true, shift: true });
        else sendKey(env, keyTarget(), 'L', 'KeyL', 76, { ctrl: true, shift: true });
        env.setTimeout(() => {
          const input = next === 'search' ? q(NAMES.searchInput) : null;
          const m = main(), el = usable(input) ? input : usable(m) ? toUnit(Nav.first(m, page.skip)) : null;
          if (el) page.moveTo(el);
        }, SECTION_MS);
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

  return { NAMES, matches, isQueue, isNeverLabel, sectionAfter, LAYOUT_WIDTH, driver };
})();

if (typeof module !== 'undefined') module.exports = OwneetSpotify;
