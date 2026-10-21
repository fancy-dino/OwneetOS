// SPDX-License-Identifier: GPL-3.0-or-later
//
// Netflix's own navigation: the geometry of its rows and where it applies ([[extension-design]]
// "Netflix"). The parts that need Netflix's pages are checked in the test VM.
// Run: node --test extension/test/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const NF = require('../content/netflix.js');

const box = (left, width) => ({ left, width, right: left + width, top: 0, bottom: 100, height: 100 });

test('only on Netflix', () => {
  for (const h of ['www.netflix.com', 'netflix.com', 'help.netflix.com']) assert.ok(NF.matches({ hostname: h }), h);
  for (const h of ['www.youtube.com', 'netflix.com.example.org', 'notnetflix.com']) assert.ok(!NF.matches({ hostname: h }), h);
});

test('up / down keep about the same column', () => {
  const row = [box(0, 180), box(190, 180), box(380, 180), box(570, 180)];
  assert.strictEqual(NF.nearestTo(90, row), 0);
  assert.strictEqual(NF.nearestTo(300, row), 1);
  assert.strictEqual(NF.nearestTo(640, row), 3);
  assert.strictEqual(NF.nearestTo(10000, row), 3);
  assert.strictEqual(NF.nearestTo(100, []), -1);
});

test('a tile is visible when fully inside its row across', () => {
  const area = { left: 0, right: 625 };
  assert.ok(NF.insideAcross(box(0, 180), area));
  assert.ok(NF.insideAcross(box(446, 180), area));          // 1 px over: the slack
  assert.ok(!NF.insideAcross(box(560, 180), area));         // cut by the edge: the row slides on
  assert.ok(!NF.insideAcross(box(-90, 180), area));
});

test('after the row slid on: the wanted title, else the first (right) or last (left) shown', () => {
  const shown = ['Dark', 'Ozark', 'Narcos'];
  assert.strictEqual(NF.afterSlide(shown, 'Ozark', 1), 1);
  assert.strictEqual(NF.afterSlide(shown, 'Elsewhere', 1), 0);
  assert.strictEqual(NF.afterSlide(shown, null, -1), 2);
  assert.strictEqual(NF.afterSlide([], 'Dark', 1), -1);
});

test("Netflix's names are valid selectors in one place", () => {
  for (const [name, sel] of Object.entries(NF.NAMES)) {
    assert.ok(typeof sel === 'string' && sel.startsWith('['), name);
    // balanced brackets and quotes: a typo would break the whole query
    assert.strictEqual((sel.match(/\[/g) || []).length, (sel.match(/\]/g) || []).length, name);
    assert.strictEqual((sel.match(/"/g) || []).length % 2, 0, name);
  }
  assert.match(NF.NAMES.card, /standard-card/);
  assert.match(NF.NAMES.card, /ranked-card/);
});
