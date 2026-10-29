// SPDX-License-Identifier: GPL-3.0-or-later
//
// The keys sent to YouTube's TV interface, and when B asks to close it ([[extension-design]]).
// Run: node --test extension/test/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const TV = require('../content/youtube-tv.js');

// A page: elements that record the events they get, a clock run by hand
function page(href = 'https://www.youtube.com/tv#/') {
  const events = [];
  const element = name => ({ name, dispatchEvent: e => events.push({ to: name, ...e }) });
  const timers = [];
  const env = {
    document: { body: element('body'), activeElement: null },
    location: { href },
    KeyboardEvent: function (type, init) { return { type, ...init }; },
    setTimeout: (f, ms) => timers.push({ f, ms }),
  };
  const run = () => timers.splice(0).forEach(t => t.f());
  return { env, events, element, timers, run };
}

test('only on the TV interface', () => {
  const at = (hostname, pathname) => TV.matches({ hostname, pathname });
  assert.ok(at('www.youtube.com', '/tv'));
  assert.ok(at('www.youtube.com', '/tv/'));
  assert.ok(!at('www.youtube.com', '/'));
  assert.ok(!at('www.youtube.com', '/tvx'));
  assert.ok(!at('m.youtube.com', '/tv'));
  assert.ok(!at('accounts.google.com', '/tv'));
});

test('the D-pad sends arrows and A sends Enter, to the focused element, with their keyCodes', () => {
  const p = page();
  p.env.document.activeElement = p.element('tile');
  const d = TV.driver(p.env);
  // every button is the TV interface's: nothing is left to the generic navigation
  for (const taken of [d.move('up'), d.move('down'), d.move('left'), d.move('right'), d.press(),
    d.page(), d.seek(1), d.scroll()]) assert.strictEqual(taken, true);
  const downs = p.events.filter(e => e.type === 'keydown').map(e => [e.key, e.keyCode, e.which, e.to]);
  assert.deepStrictEqual(downs, [['ArrowUp', 38, 38, 'tile'], ['ArrowDown', 40, 40, 'tile'],
    ['ArrowLeft', 37, 37, 'tile'], ['ArrowRight', 39, 39, 'tile'], ['Enter', 13, 13, 'tile']]);
  // every key is pressed and released, bubbling out of YouTube's components
  assert.strictEqual(p.events.filter(e => e.type === 'keyup').length, 5);
  assert.ok(p.events.every(e => e.bubbles && e.composed && e.code === e.key));
});

test('with nothing focused the keys go to the page body', () => {
  const p = page();
  TV.driver(p.env).press();
  assert.strictEqual(p.events[0].to, 'body');
});

test('B sends Escape and does not ask when YouTube went back', () => {
  const p = page();
  p.env.document.activeElement = p.element('tile');
  let asked = 0;
  TV.driver(p.env).back(() => asked++);
  assert.deepStrictEqual(p.events.map(e => [e.type, e.key, e.keyCode]), [['keydown', 'Escape', 27], ['keyup', 'Escape', 27]]);
  assert.strictEqual(p.timers[0].ms, TV.SETTLE_MS);
  p.env.document.activeElement = p.element('menu');      // the tile's row → YouTube's left menu
  p.run();
  assert.strictEqual(asked, 0);
});

test('B does not ask when only the address changed (a video closed)', () => {
  const p = page('https://www.youtube.com/tv#/watch?v=abc');
  const player = p.element('player');
  p.env.document.activeElement = player;
  let asked = 0;
  TV.driver(p.env).back(() => asked++);
  p.env.location.href = 'https://www.youtube.com/tv#/';
  p.run();
  assert.strictEqual(asked, 0);
});

test('B asks to close where Escape changed nothing (the account page)', () => {
  const p = page();
  p.env.document.activeElement = p.element('sign-in button');
  let asked = 0;
  TV.driver(p.env).back(() => asked++);
  p.run();
  assert.strictEqual(asked, 1);
});

test('two quick presses on the account page ask once', () => {
  const p = page();
  p.env.document.activeElement = p.element('sign-in button');
  let asked = 0;
  const d = TV.driver(p.env);
  d.back(() => asked++);
  d.back(() => asked++);
  p.run();
  assert.strictEqual(asked, 1);
});

test('two quick presses that go back two screens never ask', () => {
  const p = page('https://www.youtube.com/tv#/watch?v=abc');
  p.env.document.activeElement = p.element('player');
  let asked = 0;
  const d = TV.driver(p.env);
  d.back(() => asked++);
  p.env.location.href = 'https://www.youtube.com/tv#/';    // the video closed
  p.env.document.activeElement = p.element('tile');
  d.back(() => asked++);
  p.env.document.activeElement = p.element('menu');       // then the left menu
  p.run();
  assert.strictEqual(asked, 0);
});
