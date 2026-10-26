// SPDX-License-Identifier: GPL-3.0-or-later
//
// Spotify's own navigation ([[step-5.5-spotify]]; [[extension-design]] "Spotify"; bindings:
// [[controller-input-map]] "Inside Spotify"). owneetd opens Spotify with an Android TV user agent:
// Spotify then serves its player for phones and TVs (one column with horizontal shelves, a mini
// player and tabs fixed at the bottom, a full player over the page), driven part by part with the
// ring of input.js. That view has no keyboard shortcuts: the extension clicks.
//  - The targets are the page's (never the footer, links to other sites, "Get App" / "Open App",
//    nor the fixed bottom bars); a card and a song are one target each. In a shelf left / right go
//    from card to card (the shelf scrolls); up / down from a card go to the next shelf's cards,
//    about the same column. Elsewhere the nearest target in the direction. The page scrolls so that
//    the selection is never under the fixed bars.
//  - A clicks: a card through its own link, a song's row (Spotify plays it). B closes an open menu
//    or Spotify window first, then the full player, then the generic B: back a page, or "Close
//    Spotify?".
//  - X: the selected item's "More" menu, then its "Add to queue" (found by its label: the menu's
//    items have no names of their own; in another language the menu stays open). Menu: that menu.
//  - Y: the full player (a click on the mini player), the selection on its play / pause; Y or B
//    closes it. LT / RT, with the full player open: its progress bar clicked 5 s back / forward.
//  - LB / RB: the tabs Home → Search → Your Library, going round.
//  - On a TV: drawn as on a screen about 1280 px wide (layoutWidth: the worker sets Brave's zoom).
//
// Spotify's names (data-testid, read with probes in the test VM on 2026-10-26) live in NAMES; what
// is not found falls back to the generic navigation. Tests: test/spotify.test.js; the pages
// themselves are checked in the test VM.

'use strict';

var OwneetSpotify = (() => {
  const { nearestTo, insideAcross, sendKey } = typeof OwneetShared !== 'undefined'
    ? OwneetShared : require('./shared.js');

  const NAMES = {
    shelf: '[data-testid="carousel-mwp"]',
    card: '[data-testid="home-card"], [data-testid="card-mwp"]',
    song: '[data-testid="track-row"]',
    more: '[data-testid="context-menu-action-button"]',
    topBar: '[data-testid="entity-view-top-bar"]',
    miniPlayer: '[data-testid="floating-now-playing-bar"]',
    tabs: '[data-testid="bottom-tab-navigation"]',
    // the full player, over the page
    playerPlay: '[data-testid="npv-play-button"]',
    playerMore: '[data-testid="npv-context-menu"]',
    progress: '[data-testid="progress-bar"]',
    times: '[data-testid="npv-progress-bar-time-labels"]',
    footer: 'footer, [data-testid="footer-div"], [data-testid="masthead-footer"]',
    searchInput: 'input[type="search"], input[role="searchbox"], [data-testid="search-input"]',
    window: '[role="menu"], [role="dialog"], [aria-modal="true"]',
    // never reached: links Spotify keeps for a mouse, the language chooser, a download
    never: '[data-testid="mouse-only-link"], [data-testid="language-selection-button"], a[href*="/download"]',
  };
  // The labels Spotify gives the items found by their label, in the console's languages
  const QUEUE = [/^add to queue$/i, /^aggiungi (in|alla) coda$/i];
  const NEVER = [/^(open|get) (the )?app$/i, /^(apri|scarica|ottieni) (l'|la )?app$/i, /desktop app$/i, /app desktop$/i];
  const CLOSE = /^(close|cancel|chiudi|annulla)$/i;
  const TABS = ['home', 'search', 'library'];
  const LAYOUT_WIDTH = 1280;     // on a TV Spotify lays out as on a screen this wide (owner, 2026-10-24)
  const SEEK_S = 5;              // LT / RT (owner, 2026-10-26)
  const SEEK_ADD_MS = 1500;      // presses this close add up (Spotify's time labels lag behind)
  const MENU_MS = 400;           // a menu appearing
  const PLAYER_MS = 600;         // the full player opening and closing
  const TAB_MS = 700;            // a tab's page appearing
  const EDGE = 16;               // px kept between the selection and the fixed bars

  const matches = loc => loc.hostname === 'open.spotify.com';
  const isQueue = label => QUEUE.some(r => r.test(label));
  const isNeverLabel = label => NEVER.some(r => r.test(label));
  const isClose = label => CLOSE.test(label);
  // LB / RB: the tab after now (step 1) or before it (-1), going round
  const tabAfter = (now, step) => TABS[(TABS.indexOf(now) + step + TABS.length) % TABS.length];
  // "1:05" or "1:02:03" in seconds (NaN when it is not a time)
  const seconds = s => /^\d+(:\d{2}){1,2}$/.test(s) ? s.split(':').reduce((t, x) => t * 60 + Number(x), 0) : NaN;
  // Where on the progress bar (0…1) a seek of step seconds from pos lands, in a song of dur seconds
  const seekFraction = (pos, dur, step) => (dur > 0 ? Math.min(dur, Math.max(0, pos + step)) / dur : NaN);

  // env: { document, location, KeyboardEvent, MouseEvent, PointerEvent, setTimeout, now } of the
  // page; page: the ring of input.js { Nav, skip, current(), moveTo(el), click(el), openKeyboard(el) }
  function driver(env, page) {
    const doc = env.document, loc = env.location, Nav = page.Nav;
    const q = (sel, root = doc) => root.querySelector(sel);
    const qa = (sel, root = doc) => Array.from(root.querySelectorAll(sel));
    const usable = el => !!el && Nav.usable(el);
    const label = el => (el.innerText || el.getAttribute('aria-label') || '').trim().replace(/\s+/g, ' ');
    const centreX = el => { const r = Nav.rectOf(el); return r.left + r.width / 2; };
    let column = null;      // the x kept while moving up / down between shelves
    let playerFrom = null;  // where the selection was before Y opened the full player
    let menuFrom = null;    // ... before a menu or a Spotify window opened
    let seekTo = null;      // { at, pos }: the last seek, which later presses add to

    // ---- The parts
    // The full player, once open (its play button is there)
    function player() {
      const pp = q(NAMES.playerPlay);
      return usable(pp) ? (pp.closest('aside') || pp.parentElement) : null;
    }
    // Spotify keeps closed windows in the page: an open one is on screen
    function onScreen(el) {
      const r = el.getBoundingClientRect(), view = doc.defaultView;
      return r.bottom > 0 && r.right > 0 && r.top < view.innerHeight && r.left < view.innerWidth;
    }
    const openWindow = () => qa(NAMES.window).filter(w => usable(w) && onScreen(w)).pop() || null;
    // Spotify's player view: its shelves, songs, mini player or tabs are there
    const playerView = () => !!q(`${NAMES.tabs}, ${NAMES.miniPlayer}, ${NAMES.shelf}, ${NAMES.song}`);
    const fixedBottom = () => [q(NAMES.miniPlayer), q(NAMES.tabs)].filter(usable);
    const unitOf = el => (el && el.closest ? el.closest(`${NAMES.card}, ${NAMES.song}`) : null);
    const toUnit = el => unitOf(el) || el;
    function external(el) {
      if (!el.matches('a[href]')) return false;
      try { return new URL(el.href, loc.href).hostname !== loc.hostname; } catch (_) { return true; }
    }
    function isNever(el) {
      if (el.closest(NAMES.never) || external(el)) return true;
      return !unitOf(el) && el.matches('a, button, [role="button"]') && isNeverLabel(label(el));
    }

    // The D-pad's targets in the page: not in the footer, the fixed bottom bars or the full
    // player; a card or a song counts once
    function targets() {
      const skip = [...fixedBottom(), ...qa(NAMES.footer)];
      const pl = player();
      if (pl) skip.push(pl);
      const seen = new Set(), out = [];
      for (const el of Nav.targets(doc, page.skip, true)) {
        if (skip.some(s => s.contains(el)) || isNever(el)) continue;
        const t = toUnit(el);
        if (seen.has(t) || !usable(t)) continue;
        seen.add(t);
        out.push(t);
      }
      return out;
    }
    // The nearest target in dir; outside: an element whose targets are not candidates
    function nearest(cur, dir, list, outside) {
      const els = [], cands = [];
      for (const el of list) {
        if (el === cur || el.contains(cur) || cur.contains(el) || (outside && outside.contains(el))) continue;
        els.push(el);
        cands.push({ rect: Nav.rectOf(el), level: 0, sameRow: true });
      }
      const i = Nav.pick(Nav.rectOf(cur), cands, dir, null);
      return i < 0 ? null : els[i];
    }
    // The selection, with the page scrolled so that it is not under the fixed bars
    function select(el) {
      const r = el.getBoundingClientRect();
      const bar = q(NAMES.topBar), top = usable(bar) ? bar.getBoundingClientRect().bottom : 0;
      const bottom = Math.min(doc.defaultView.innerHeight, ...fixedBottom().map(b => b.getBoundingClientRect().top));
      let dy = 0;
      if (r.bottom > bottom - EDGE) dy = r.bottom - (bottom - EDGE);
      if (r.top - dy < top + EDGE) dy = r.top - (top + EDGE);
      if (dy) doc.scrollingElement.scrollBy({ top: dy });
      page.moveTo(el);
    }

    // ---- Shelves (they scroll sideways by themselves)
    const shelfOf = el => (el && el.closest && el.closest(NAMES.shelf)) || null;
    const cardsIn = shelf => qa(NAMES.card, shelf).filter(usable);
    function toShelf(shelf, cur) {
      if (column === null) column = centreX(cur);
      const area = shelf.getBoundingClientRect(), cards = cardsIn(shelf);
      const shown = cards.filter(c => insideAcross(Nav.rectOf(c), area)), pool = shown.length ? shown : cards;
      const j = nearestTo(column, pool.map(c => Nav.rectOf(c)));
      if (j >= 0) select(pool[j]);
      return j >= 0;
    }

    // ---- Moving in the page
    function browse(dir, again) {
      if (!playerView()) return false;                  // not Spotify's player: the generic navigation
      const list = targets();
      const cur = toUnit(page.current());
      if (!cur || !list.includes(cur)) {
        const first = list.find(el => onScreen(el)) || list[0];
        if (first) select(first);
        return true;
      }
      const sideways = dir === 'left' || dir === 'right';
      const shelf = cur.matches(NAMES.card) ? shelfOf(cur) : null;
      if (sideways) {
        column = null;
        if (shelf) {
          const cards = cardsIn(shelf), next = cards[cards.indexOf(cur) + (dir === 'right' ? 1 : -1)];
          if (next) { select(next); return true; }
          return true;                                  // the shelf's end
        }
      }
      // up / down from a card skip the rest of its shelf
      const el = nearest(cur, dir, list, shelf && !sideways ? shelf : null);
      if (!el) {
        // nothing more: the page scrolls (Spotify draws long lists as they come), then once more
        if (!sideways && !again) {
          doc.scrollingElement.scrollBy({ top: (dir === 'down' ? 0.4 : -0.4) * doc.defaultView.innerHeight });
          env.setTimeout(() => browse(dir, true), 300);
        }
        return true;
      }
      const other = shelfOf(el);
      if (!sideways && other && other !== shelf && toShelf(other, cur)) return true;
      column = null;
      select(el);
      return true;
    }

    // ---- Menus, Spotify's windows and the full player: the selection stays inside while open
    // remember: B brings the selection back where it was
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
    function enter(root, first) {
      const el = usable(first) ? first : Nav.targets(root, page.skip).find(t => !isNever(t));
      if (el) page.moveTo(el);
    }
    // The selection back where it was before a menu or a window
    function backFrom(ms) {
      const from = menuFrom;
      menuFrom = null;
      env.setTimeout(() => { if (usable(from)) select(from); }, ms);
    }
    // An item's "More" menu (its own button, else as a right click), then `then` once it is open
    function openMenuFor(el, then) {
      menuFrom = el;
      const more = unitOf(el) ? q(NAMES.more, unitOf(el)) : null;
      if (usable(more)) page.click(more);
      else {
        const r = el.getBoundingClientRect();
        el.dispatchEvent(new env.MouseEvent('contextmenu', { bubbles: true, cancelable: true, composed: true,
          view: doc.defaultView, button: 2, buttons: 2, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2 }));
      }
      env.setTimeout(() => { const m = openWindow(); if (m) then(m); }, MENU_MS);
    }
    function closeWindow(w) {
      sendKey(env, doc.activeElement || w, 'Escape', 'Escape', 27);
      env.setTimeout(() => {
        if (!usable(w) || !onScreen(w)) return;
        const close = qa('button, [role="button"]', w).find(b => usable(b) && isClose(label(b)));
        if (close) page.click(close);
      }, 250);
      backFrom(350);
    }
    function openPlayer() {
      const mini = q(NAMES.miniPlayer);
      if (!usable(mini)) return;                        // nothing playing
      playerFrom = page.current();
      page.click(mini);
      env.setTimeout(() => { const pl = player(); if (pl) enter(pl, q(NAMES.playerPlay)); }, PLAYER_MS);
    }
    function closePlayer(pl) {
      // its Close button: by its label, else the first one (top left)
      const buttons = qa('button, [role="button"]', pl).filter(usable);
      const close = buttons.find(b => isClose(label(b))) || buttons[0];
      if (close) page.click(close);
      const from = playerFrom;
      playerFrom = null;
      env.setTimeout(() => { if (usable(from)) select(from); }, PLAYER_MS);
    }
    // LT / RT in the full player: a click on its progress bar, step seconds from where it is
    function seek(pl, step) {
      const bar = q(NAMES.progress, pl), labels = q(NAMES.times, pl);
      if (!usable(bar) || !labels) return;
      const [pos, dur] = labels.innerText.split(/\s+/).map(seconds).filter(n => !Number.isNaN(n));
      if (!(dur > 0)) return;
      const from = seekTo && env.now() - seekTo.at < SEEK_ADD_MS ? seekTo.pos : pos;
      const f = seekFraction(from, dur, step * SEEK_S);
      seekTo = { at: env.now(), pos: f * dur };
      const r = bar.getBoundingClientRect(), x = r.left + f * r.width, y = r.top + r.height / 2;
      const target = doc.elementFromPoint(x, y) || bar;
      const init = { bubbles: true, cancelable: true, composed: true, view: doc.defaultView,
        clientX: x, clientY: y, screenX: x, screenY: y, button: 0 };
      target.dispatchEvent(new env.PointerEvent('pointerdown', { ...init, pointerType: 'mouse', isPrimary: true, buttons: 1 }));
      target.dispatchEvent(new env.MouseEvent('mousedown', { ...init, buttons: 1 }));
      target.dispatchEvent(new env.PointerEvent('pointerup', { ...init, pointerType: 'mouse', isPrimary: true }));
      target.dispatchEvent(new env.MouseEvent('mouseup', init));
      target.dispatchEvent(new env.MouseEvent('click', init));
    }
    // The tabs' own links (not "Get App"), in their order: Home, Search, Your Library
    const tabLinks = () => { const t = q(NAMES.tabs); return t ? qa('a[href]', t).filter(a => usable(a) && !isNever(a)) : []; };

    return {
      layoutWidth: LAYOUT_WIDTH,
      move(dir) {
        const w = openWindow();
        if (w) return inside(w, dir, true);
        menuFrom = null;                               // closed by itself (e.g. one of its items went to a page)
        const pl = player(), cur = page.current();
        if (pl && !(cur && pl.contains(cur))) { enter(pl, q(NAMES.playerPlay)); return true; }
        if (pl) return inside(pl, dir, false);
        return browse(dir, false);
      },
      press() {
        const w = openWindow() || player(), cur = page.current();
        if (w && !(cur && w.contains(cur))) {
          if (!menuFrom && w !== player()) menuFrom = cur;
          enter(w, w === player() ? q(NAMES.playerPlay) : null);
          return true;
        }
        const unit = unitOf(cur);
        if (unit && unit.matches(NAMES.card)) {
          // a card opens through its own link
          const link = qa('a[href]', unit).find(usable);
          page.click(link || unit);
          return true;
        }
        return false;                                  // the generic A clicks it (a song's row plays it)
      },
      back() {
        if (doc.fullscreenElement) return false;       // the generic B leaves full screen
        const w = openWindow();
        if (w) { closeWindow(w); return true; }
        const pl = player();
        if (pl) { closePlayer(pl); return true; }
        return false;                                  // the generic B: back a page, or "Close Spotify?"
      },
      // X: the selected item into the queue (its "More" menu's "Add to queue")
      secondary() {
        const unit = unitOf(page.current());
        if (!unit || openWindow() || player()) return true;
        openMenuFor(unit, m => {
          const item = qa('[role="menuitem"], button, [role="button"], li', m).find(it => usable(it) && isQueue(label(it)));
          if (!item) { enter(m); return; }             // another language: the menu stays open
          page.click(item);
          backFrom(350);
        });
        return true;
      },
      // Menu: the selected item's "More" menu (in the full player, the player's), the selection inside
      options() {
        if (openWindow()) return true;
        const pl = player(), cur = page.current();
        if (pl) {
          const more = q(NAMES.playerMore, pl);
          if (!usable(more)) return true;
          menuFrom = cur;
          page.click(more);
          env.setTimeout(() => { const m = openWindow(); if (m) enter(m); }, MENU_MS);
          return true;
        }
        if (cur) openMenuFor(toUnit(cur), m => enter(m));
        return true;
      },
      // Y: the full player and back
      page() {
        if (openWindow()) return true;
        const pl = player();
        if (pl) closePlayer(pl);
        else openPlayer();
        return true;
      },
      // LT / RT: back / forward 5 s with the full player open, nothing elsewhere
      seek(step) {
        const pl = player();
        if (pl && !openWindow()) seek(pl, step);
        return true;
      },
      // LB / RB: the tabs Home → Search → Your Library, going round
      section(step) {
        if (openWindow() || player()) return true;
        const links = tabLinks();
        if (links.length < TABS.length) return true;
        const at = links.findIndex(a => new URL(a.href, loc.href).pathname === loc.pathname);
        const now = at >= 0 ? TABS[at] : loc.pathname.startsWith('/search') ? 'search' : 'home';
        const next = tabAfter(now, step);
        column = null;
        page.click(links[TABS.indexOf(next)]);
        env.setTimeout(() => {
          const input = next === 'search' ? qa(NAMES.searchInput).find(usable) : null;
          if (input) { select(input); return; }
          const first = targets().find(el => onScreen(el));
          if (first) select(first);
        }, TAB_MS);
        return true;
      },
      // the right stick: the page up / down (nothing in the full player), a shelf sideways
      scroll(dx, dy) {
        if (player()) return true;
        const cur = page.current(), shelf = shelfOf(cur);
        if (dy) doc.scrollingElement.scrollBy({ top: dy });
        if (dx && shelf) shelf.scrollBy({ left: dx });
        return true;
      },
    };
  }

  return { NAMES, matches, isQueue, isNeverLabel, isClose, tabAfter, seconds, seekFraction, LAYOUT_WIDTH, driver };
})();

if (typeof module !== 'undefined') module.exports = OwneetSpotify;
