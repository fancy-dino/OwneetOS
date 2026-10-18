// SPDX-License-Identifier: GPL-3.0-or-later
//
// YouTube's TV interface (youtube.com/tv, [[step-5.3-youtube]]; [[extension-design]] "YouTube's TV
// interface"; bindings: [[controller-input-map]] "Inside YouTube"). It does not read the controller:
// it is made for a remote and takes keyboard keys, also sent by a script (checked in the test VM).
// So on its address the gamepad loop (input.js) sends it keys instead of moving the ring: D-pad =
// arrows, A = Enter, B = Escape. YouTube draws its own selection: no ring.
//
// Escape goes back up to YouTube's "Welcome back" account page and there does nothing; that page
// has the same address as the home grid, so B asks to close the web app when Escape changed
// nothing: neither the address nor the focused element, half a second later.
//
// The page's objects come in `env`, so that the tests can run without a browser
// (test/youtube-tv.test.js).

'use strict';

var OwneetTV = (() => {
  // [key, keyCode]: the TV interface reads keyCode (Brave keeps it from the event's init dictionary)
  const KEYS = {
    up: ['ArrowUp', 38], down: ['ArrowDown', 40], left: ['ArrowLeft', 37], right: ['ArrowRight', 39],
    enter: ['Enter', 13], escape: ['Escape', 27],
  };
  // How long YouTube has to react to Escape before "nothing changed"
  const SETTLE_MS = 500;

  function matches(loc) {
    return loc.hostname === 'www.youtube.com' && (loc.pathname === '/tv' || loc.pathname.startsWith('/tv/'));
  }

  // env: { document, location, KeyboardEvent, setTimeout } of the page (fakes in the tests)
  function driver(env) {
    // keydown and keyup on the focused element, as a keyboard sends them
    function send(name) {
      const [key, keyCode] = KEYS[name];
      const target = env.document.activeElement || env.document.body;
      for (const type of ['keydown', 'keyup']) {
        target.dispatchEvent(new env.KeyboardEvent(type, { key, code: key, keyCode, which: keyCode,
          bubbles: true, cancelable: true, composed: true }));
      }
    }

    // Only the last B of a quick series may ask: two presses on the account page ask once, and
    // two presses that go back two screens never ask
    let presses = 0;
    return {
      move: dir => send(dir),
      press: () => send('enter'),
      // B: YouTube's back; where it has nothing left, onNothingLeft (the console's "Close YouTube?")
      back(onNothingLeft) {
        const href = env.location.href, focus = env.document.activeElement, mine = ++presses;
        send('escape');
        env.setTimeout(() => {
          if (mine === presses && env.location.href === href && env.document.activeElement === focus) onNothingLeft();
        }, SETTLE_MS);
      },
    };
  }

  return { matches, driver, SETTLE_MS };
})();

if (typeof module !== 'undefined') module.exports = OwneetTV;
