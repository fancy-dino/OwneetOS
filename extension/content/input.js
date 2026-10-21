// SPDX-License-Identifier: GPL-3.0-or-later
//
// The controller inside a web app ([[controller-input-map]] "Inside a web app",
// [[extension-design]]): D-pad / left stick move a ring between the page's targets, A presses,
// B leaves full screen / closes the page's dialog / goes back / asks to close the web app, the
// right stick scrolls. Guide (standard button 16) is owneetd's and is never read. Runs in the top
// frame only; the console's keyboard and "Close {name}?" go through the background worker and the
// bridge to owneetd. A supported web app has a navigation of its own, which takes the buttons it
// wants ([[extension-design]] "One navigation per supported app"): YouTube's TV interface
// (youtube-tv.js, "Inside YouTube"), Netflix (netflix.js, "Inside Netflix").

'use strict';

(() => {
  if (window.top !== window) return;
  const Nav = OwneetNav;

  // Standard Gamepad API mapping
  const BTN = { a: 0, b: 1, y: 3, lt: 6, rt: 7, up: 12, down: 13, left: 14, right: 15 };
  const REPEAT_FIRST = 360, REPEAT_NEXT = 140;     // held directions ([[controller-input-map]])
  const TRIGGER_FIRST = 720, TRIGGER_NEXT = 360;   // held LT / RT
  const TRIGGER_ON = 0.5, TRIGGER_OFF = 0.25;      // analog triggers: pressed past half, released under a quarter
  const STICK = 0.5;                               // left stick: a direction past half its travel
  const SCROLL_DEAD = 0.2, SCROLL_SPEED = 22;      // right stick: px per frame at full tilt
  const RING_IDLE_FULLSCREEN = 3000;               // in full screen the ring hides after 3 s idle

  // ---------- Bridge (background worker → owneet-bridge → owneetd) ----------
  let port = null, paused = false;
  function connect() {
    try {
      port = chrome.runtime.connect({ name: 'page' });
    } catch (_) { port = null; return; }
    port.onMessage.addListener(onBridge);
    port.onDisconnect.addListener(() => { port = null; setTimeout(connect, 2000); });
  }
  const send = msg => { if (port) port.postMessage(msg); };
  function onBridge(msg) {
    switch (msg.type) {
      case 'look':
        if (msg.accent) ring.style.setProperty('--owneet-accent', msg.accent);
        break;
      case 'text': Keyboard.insert(msg.text); break;
      case 'key': Keyboard.key(msg.key); break;
      case 'keyboard_closed':
        paused = false; settle();
        if (msg.next) moveTo(Nav.next(document, current || lastRect, 'down', ringHost));
        break;
    }
  }

  // ---------- The ring: one box over the selected element (the site's styles are untouched) ----------
  const ringHost = document.createElement('owneet-ring');
  const shadow = ringHost.attachShadow({ mode: 'closed' });
  shadow.innerHTML = `<style>
    :host { all: initial; }
    div { position: fixed; pointer-events: none; z-index: 2147483647; display: none; box-sizing: border-box;
          box-shadow: 0 0 0 3px rgba(0,0,0,.65), 0 0 0 6.5px var(--owneet-accent, Highlight),
                      0 0 24px 4px color-mix(in srgb, var(--owneet-accent, Highlight) 45%, transparent);
          transition: left .1s ease, top .1s ease, width .1s ease, height .1s ease; }
    @media (prefers-reduced-motion: reduce) { div { transition: none; } }
  </style><div></div>`;
  const ring = shadow.querySelector('div');
  // anchor: the column (or row) kept while moving on one axis (OwneetNav.anchorAfter)
  let current = null, lastRect = null, lastInput = 0, drawn = '', anchor = null;

  function attachRing() {
    // In full screen only the full-screen element's subtree is shown (not a <video>: no children)
    const fs = document.fullscreenElement;
    const parent = fs && !(fs instanceof HTMLMediaElement) ? fs : document.documentElement;
    if (ringHost.parentNode !== parent) parent.appendChild(ringHost);
  }

  function drawRing(now) {
    attachRing();
    const idle = document.fullscreenElement && now - lastInput > RING_IDLE_FULLSCREEN;
    // The ring stays around the field while the console's keyboard types into it (drawn over the
    // page in gamescope, as in the approved demo; owner, 2026-10-17)
    const show = current && Nav.usable(current) && !idle && !(site && site.hideRing && site.hideRing());
    const r = show ? Nav.rectOf(current) : null;
    const key = r ? `${r.left}|${r.top}|${r.width}|${r.height}` : '';
    if (key === drawn) return;
    drawn = key;
    if (!r) { ring.style.display = 'none'; return; }
    const radius = parseFloat(getComputedStyle(current).borderTopLeftRadius) || 4;
    Object.assign(ring.style, { display: 'block', left: r.left + 'px', top: r.top + 'px', width: r.width + 'px',
      height: r.height + 'px', borderRadius: Math.max(radius, 4) + 'px' });
  }

  // ---------- Moving ----------
  const reduced = () => matchMedia('(prefers-reduced-motion: reduce)').matches;

  // The new target is scrolled fully into view: its scrolling containers, then the page (with a
  // margin of 15% of the screen, so that sticky headers do not hide it).
  function reveal(el) {
    const behavior = reduced() ? 'auto' : 'smooth';
    for (let s = Nav.scroller(el); s;) {
      const page = s === s.ownerDocument.scrollingElement;
      const r = el.getBoundingClientRect();
      const v = page ? { left: 0, top: 0, right: innerWidth, bottom: innerHeight } : s.getBoundingClientRect();
      const mx = page ? 0.05 * innerWidth : 24, my = page ? 0.15 * innerHeight : 24;
      const dx = r.left - mx < v.left ? r.left - mx - v.left : r.right + mx > v.right ? r.right + mx - v.right : 0;
      const dy = r.top - my < v.top ? r.top - my - v.top : r.bottom + my > v.bottom ? r.bottom + my - v.bottom : 0;
      if (dx || dy) s.scrollBy({ left: dx, top: dy, behavior });
      if (page) break;
      s = Nav.scroller(s);
    }
  }

  function moveTo(el) {
    if (!el) return;
    current = el;
    lastRect = Nav.rectOf(el);
    // Elements reached through their role only get tabindex="-1" so that they can take focus
    if (!el.hasAttribute('tabindex') && !el.matches('a[href], button, input, select, textarea, summary, [contenteditable]')) {
      el.setAttribute('tabindex', '-1');
    }
    el.focus({ preventScroll: true });
    reveal(el);
  }

  // The selection still on the page (sites replace their elements while they update)
  function settle() {
    if (current && Nav.usable(current)) return true;
    current = null;
    const active = document.activeElement;
    if (active && active !== document.body && Nav.usable(active) && active.matches(Nav.SELECTOR)) { current = active; return true; }
    return false;
  }

  function move(dir) {
    if (!settle()) {
      // Gone (or never chosen): continue from where the last one was, else from the first one
      const el = lastRect ? Nav.next(document, lastRect, dir, ringHost, anchor) : null;
      moveTo(el || Nav.first(document, ringHost));
      return;
    }
    const from = Nav.rectOf(current);
    const el = Nav.next(document, current, dir, ringHost, anchor);
    if (el) {
      anchor = Nav.anchorAfter(anchor, from, dir);
      moveTo(el);
      return;
    }
    // Nothing in that direction: the row (sideways) or the page scrolls one step
    const horiz = dir === 'left' || dir === 'right';
    const s = horiz ? Nav.scroller(current) : document.scrollingElement;
    const step = 0.4 * (horiz ? s.clientWidth : innerHeight);
    s.scrollBy({ left: dir === 'right' ? step : dir === 'left' ? -step : 0,
      top: dir === 'down' ? step : dir === 'up' ? -step : 0, behavior: reduced() ? 'auto' : 'smooth' });
  }

  // ---------- A ----------
  const TEXT_TYPES = ['text', 'email', 'password', 'search', 'url', 'tel', 'number'];
  function isTextField(el) {
    if (el instanceof HTMLTextAreaElement) return !el.readOnly;
    if (el instanceof HTMLInputElement) return TEXT_TYPES.includes(el.type) && !el.readOnly;   // el.type: "text" when unknown
    return el.isContentEditable;
  }

  function press() {
    anchor = null;
    if (!settle()) { moveTo(Nav.first(document, ringHost)); return; }
    const el = current;
    if (isTextField(el)) { el.focus({ preventScroll: true }); Keyboard.open(el); return; }
    clickAt(el);
  }
  // The events a mouse click fires, at the element's centre
  function clickAt(el) {
    const r = el.getBoundingClientRect(), x = r.left + r.width / 2, y = r.top + r.height / 2;
    const opts = { bubbles: true, cancelable: true, composed: true, view: window, clientX: x, clientY: y, button: 0 };
    el.dispatchEvent(new PointerEvent('pointerdown', { ...opts, pointerType: 'mouse', isPrimary: true, buttons: 1 }));
    el.dispatchEvent(new MouseEvent('mousedown', { ...opts, buttons: 1 }));
    el.dispatchEvent(new PointerEvent('pointerup', { ...opts, pointerType: 'mouse', isPrimary: true }));
    el.dispatchEvent(new MouseEvent('mouseup', opts));
    el.click();
  }

  // ---------- B ----------
  function openDialog() {
    const list = Array.from(document.querySelectorAll('dialog[open], [aria-modal="true"]')).filter(Nav.usable);
    return list[list.length - 1] || null;
  }
  function escape(target) {
    for (const type of ['keydown', 'keyup']) {
      target.dispatchEvent(new KeyboardEvent(type, { key: 'Escape', code: 'Escape', keyCode: 27, which: 27, bubbles: true, cancelable: true, composed: true }));
    }
  }
  function back() {
    if (document.fullscreenElement) { document.exitFullscreen().catch(() => {}); return; }
    const dialog = openDialog();
    if (dialog) {
      escape(document.activeElement || dialog);
      if (dialog instanceof HTMLDialogElement && dialog.open) {
        if (dialog.requestClose) dialog.requestClose(); else dialog.close();
      }
      return;
    }
    const canGoBack = window.navigation ? window.navigation.canGoBack : history.length > 1;
    if (canGoBack) { history.back(); return; }
    send({ type: 'ask_close' });
  }

  // ---------- The console's keyboard for text fields ----------
  const Keyboard = {
    field: null,
    open(el) {
      this.field = el;
      paused = true;
      drawn = '';
      this.caretToEnd(el);
      const kind = el instanceof HTMLInputElement ? (el.type || 'text') : 'text';
      const label = (el.labels && el.labels[0] && el.labels[0].textContent.trim()) || el.getAttribute('aria-label') || el.placeholder || '';
      send({ type: 'keyboard', open: true, kind, label, value: 'value' in el ? String(el.value) : el.textContent });
      if (!port) { paused = false; }       // no bridge: nothing would bring the controller back
    },
    // The console's line starts with its cursor at the end: so does the field
    caretToEnd(el) {
      try {
        if (el.isContentEditable) {
          const range = document.createRange();
          range.selectNodeContents(el);
          range.collapse(false);
          const sel = getSelection();
          sel.removeAllRanges();
          sel.addRange(range);
        } else if (typeof el.value === 'string') {
          el.setSelectionRange(el.value.length, el.value.length);
        }
      } catch (_) { /* email and number fields have no selection to set */ }
    },
    insert(text) {
      if (!this.field) return;
      this.field.focus({ preventScroll: true });
      document.execCommand('insertText', false, text);
    },
    key(name) {
      if (!this.field) return;
      this.field.focus({ preventScroll: true });
      if (name === 'backspace') document.execCommand('delete', false);
      else if (name === 'left' || name === 'right') {
        const sel = getSelection();
        if (sel && sel.modify) sel.modify('move', name === 'left' ? 'backward' : 'forward', 'character');
      }
    },
  };

  // ---------- Gamepad loop ----------
  const prev = {};
  let heldDir = null, repeatAt = 0;
  function pad() {
    const pads = navigator.getGamepads ? Array.from(navigator.getGamepads()).filter(Boolean) : [];
    return pads.find(p => p.mapping === 'standard') || pads[0] || null;
  }
  // The direction held on the D-pad or the left stick, or null
  function direction(p, on) {
    const ax = p.axes[0] || 0, ay = p.axes[1] || 0;
    if (on(BTN.left) || ax < -STICK) return 'left';
    if (on(BTN.right) || ax > STICK) return 'right';
    if (on(BTN.up) || ay < -STICK) return 'up';
    if (on(BTN.down) || ay > STICK) return 'down';
    return null;
  }
  // LT / RT as one value: -1 (LT), 1 (RT) or 0, analog triggers with their two thresholds
  const triggerDown = {};
  function trigger(p) {
    const down = i => {
      const v = p.buttons[i] ? p.buttons[i].value : 0;
      triggerDown[i] = triggerDown[i] ? v > TRIGGER_OFF : v > TRIGGER_ON;
      return triggerDown[i];
    };
    const l = down(BTN.lt), r = down(BTN.rt);
    return l === r ? 0 : l ? -1 : 1;
  }
  let heldTrigger = 0, triggerAt = 0;
  let active = false;     // the page had the controller in the previous frame
  function frame(now) {
    const p = document.visibilityState === 'visible' && document.hasFocus() ? pad() : null;
    if (p && !paused) {
      const on = i => !!(p.buttons[i] && p.buttons[i].pressed);
      if (!active) {
        // The controller comes back (the console's keyboard closed, the page in front again):
        // buttons still held from the console count only once released and pressed again (B
        // that closed the keyboard must not also go back), a held direction or trigger waits to
        // repeat
        for (const i of [BTN.a, BTN.b, BTN.y]) prev[i] = on(i);
        heldDir = direction(p, on);
        repeatAt = now + REPEAT_FIRST;
        heldTrigger = trigger(p);
        triggerAt = now + TRIGGER_FIRST;
        active = true;
      }
      const edge = i => { const d = on(i), was = prev[i]; prev[i] = d; return d && !was; };
      const rx = p.axes[2] || 0, ry = p.axes[3] || 0;
      // A supported app's own navigation first; what it leaves goes to the generic one
      const dir = direction(p, on);
      if (dir && (dir !== heldDir || now >= repeatAt)) {
        if (!(site && site.move(dir))) move(dir);
        repeatAt = now + (dir === heldDir ? REPEAT_NEXT : REPEAT_FIRST);
        lastInput = now;
      }
      heldDir = dir;
      if ((Math.abs(rx) > SCROLL_DEAD || Math.abs(ry) > SCROLL_DEAD) && !(site && site.scroll())) {
        const s = current && Nav.usable(current) ? Nav.scroller(current) : document.scrollingElement;
        s.scrollBy({ left: Math.abs(rx) > SCROLL_DEAD ? rx * SCROLL_SPEED : 0, top: Math.abs(ry) > SCROLL_DEAD ? ry * SCROLL_SPEED : 0 });
        lastInput = now;
      }
      if (edge(BTN.a)) { if (!(site && site.press())) press(); lastInput = now; }
      if (edge(BTN.b)) { if (!(site && site.back(() => send({ type: 'ask_close' })))) back(); lastInput = now; }
      // Y and LT / RT: only a supported app gives them a meaning
      if (edge(BTN.y) && site) { site.page(); lastInput = now; }
      const t = trigger(p);
      if (t && (t !== heldTrigger || now >= triggerAt)) {
        if (site) site.seek(t);
        triggerAt = now + (t === heldTrigger ? TRIGGER_NEXT : TRIGGER_FIRST);
        lastInput = now;
      }
      heldTrigger = t;
    } else {
      active = false;
    }
    drawRing(now);
    requestAnimationFrame(frame);
  }

  // A supported web app with a navigation of its own; each of its methods answers true when it
  // took the button, false to leave it to the generic navigation
  const env = { document, location, KeyboardEvent, MouseEvent,
    setTimeout: (f, ms) => setTimeout(f, ms), now: () => performance.now() };
  const site = OwneetTV.matches(location) ? OwneetTV.driver(env)
    : OwneetNetflix.matches(location) ? OwneetNetflix.driver(env, {
      Nav, skip: ringHost, current: () => (settle() ? current : null), moveTo, click: clickAt,
      openKeyboard: el => { moveTo(el); el.focus({ preventScroll: true }); Keyboard.open(el); },
    })
    : null;

  connect();
  requestAnimationFrame(frame);
})();
