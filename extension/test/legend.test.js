// SPDX-License-Identifier: GPL-3.0-or-later
//
// The command legend ([[step-5.6a-command-legend]]): the legends the console draws (the theme's
// legends.js, one source) agree with what the web apps' navigations do: every legend a navigation
// names exists, the screen it names follows what is on screen (Netflix), and each app's legends
// name the buttons its navigation takes, and only those.
// Run: node --test extension/test/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const LEGENDS = require('../../frontend/owneet-frontend/src/themes/owneet/foundation/legends.js');
const TV = require('../content/youtube-tv.js');
const NF = require('../content/netflix.js');
const SP = require('../content/spotify.js');

// A page with only the elements named by these selectors (usable ones, at a place)
function fakePage(present, pathname = '/browse') {
  const el = sel => ({ sel, ok: true, getBoundingClientRect: () => ({ left: 0, top: 0, width: 10, height: 10 }) });
  const document = {
    querySelector: sel => (present.includes(sel) ? el(sel) : null),
    querySelectorAll: sel => (present.includes(sel) ? [el(sel)] : []),
    getElementById: () => null,
    body: {},
  };
  const env = { document, location: { pathname, href: 'https://example.org' + pathname }, setTimeout: () => 0, now: () => 0 };
  const page = { Nav: { usable: e => !!e && e.ok, rectOf: e => e.getBoundingClientRect() }, skip: null,
    current: () => null, moveTo: () => {}, click: () => {}, openKeyboard: () => {} };
  return { env, page };
}

test('every legend has rows of buttons and a message', () => {
  for (const [name, rows] of Object.entries(LEGENDS)) {
    assert.ok(rows.length > 0, name);
    for (const [buttons, key] of rows) {
      assert.ok(Array.isArray(buttons), name);
      assert.match(key, /^legend\./, name);
    }
  }
});

test("Netflix's legend follows its screens", () => {
  const at = (present, path) => { const { env, page } = fakePage(present, path); return NF.driver(env, page).legend(); };
  assert.strictEqual(at([NF.NAMES.player], '/watch/81234'), 'netflix.player');
  assert.strictEqual(at([NF.NAMES.title, NF.NAMES.row]), 'netflix.title');
  assert.strictEqual(at([NF.NAMES.row]), 'netflix.browse');
  assert.strictEqual(at([]), 'web');                       // its start page, sign-in: the web app's
  for (const c of ['netflix.player', 'netflix.title', 'netflix.browse', 'web']) assert.ok(LEGENDS[c], c);
});

test("Spotify's and YouTube's legends", () => {
  let { env, page } = fakePage([SP.NAMES.bar]);
  assert.strictEqual(SP.driver(env, page).legend(), 'spotify');
  ({ env, page } = fakePage([]));
  assert.strictEqual(SP.driver(env, page).legend(), 'web');
  assert.strictEqual(TV.driver(fakePage([]).env).legend(), 'youtube');
  assert.ok(LEGENDS.spotify && LEGENDS.youtube && LEGENDS.web);
});

// A legend's button → the navigation's method that takes it
const METHOD = { dpad: 'move', ls: 'move', a: 'press', b: 'back', x: 'secondary', y: 'page', menu: 'options',
  lt: 'seek', rt: 'seek', r3: 'stickPress', rs: 'scroll' };
// Methods every navigation has, also where they do nothing (YouTube's Y): not checked backwards
const ALWAYS = ['page', 'seek', 'scroll'];

test("each app's legends name the buttons its navigation takes, and only those", () => {
  const { env, page } = fakePage([]);
  const apps = {
    youtube: { driver: TV.driver(env), legends: ['youtube'] },
    netflix: { driver: NF.driver(env, page), legends: ['netflix.browse', 'netflix.title', 'netflix.player'] },
    spotify: { driver: SP.driver(env, page), legends: ['spotify'] },
  };
  for (const [name, { driver, legends }] of Object.entries(apps)) {
    const named = new Set();
    for (const c of legends) for (const [buttons] of LEGENDS[c]) buttons.forEach(b => named.add(b));
    for (const b of named) {
      const method = METHOD[b];
      // the right stick scrolls through the generic navigation where the app leaves it
      if (b === 'rs') continue;
      assert.ok(method, `${name}: ${b} is not a button of a web app`);
      assert.strictEqual(typeof driver[method], 'function', `${name}'s legend names ${b}, which its navigation does not take`);
    }
    for (const [b, method] of Object.entries(METHOD)) {
      if (ALWAYS.includes(method) || typeof driver[method] !== 'function') continue;
      assert.ok(named.has(b), `${name}'s navigation takes ${b} (${method}), its legend does not say so`);
    }
  }
  // the generic navigation (a web app without one of its own): D-pad, A, B, the right stick
  assert.deepStrictEqual([...new Set(LEGENDS.web.flatMap(r => r[0]))].sort(), ['a', 'b', 'dpad', 'ls', 'rs']);
});
