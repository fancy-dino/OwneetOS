// SPDX-License-Identifier: GPL-3.0-or-later
//
// What the supported web apps' navigations share: the geometry of rows of tiles that slide on
// (Netflix's rows) and keys sent as a keyboard sends them ([[extension-design]]).
// Run: node --test extension/test/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const S = require('../content/shared.js');

const box = (left, width) => ({ left, width, right: left + width, top: 0, bottom: 100, height: 100 });

test('up / down keep about the same column', () => {
  const row = [box(0, 180), box(190, 180), box(380, 180), box(570, 180)];
  assert.strictEqual(S.nearestTo(90, row), 0);
  assert.strictEqual(S.nearestTo(300, row), 1);
  assert.strictEqual(S.nearestTo(640, row), 3);
  assert.strictEqual(S.nearestTo(10000, row), 3);
  assert.strictEqual(S.nearestTo(100, []), -1);
});

test('a tile is visible when fully inside its row across', () => {
  const area = { left: 0, right: 625 };
  assert.ok(S.insideAcross(box(0, 180), area));
  assert.ok(S.insideAcross(box(446, 180), area));          // 1 px over: the slack
  assert.ok(!S.insideAcross(box(560, 180), area));         // cut by the edge: the row slides on
  assert.ok(!S.insideAcross(box(-90, 180), area));
});

test('after the row slid on: the wanted title, else the first (right) or last (left) shown', () => {
  const shown = ['Dark', 'Ozark', 'Narcos'];
  assert.strictEqual(S.afterSlide(shown, 'Ozark', 1), 1);
  assert.strictEqual(S.afterSlide(shown, 'Elsewhere', 1), 0);
  assert.strictEqual(S.afterSlide(shown, null, -1), 2);
  assert.strictEqual(S.afterSlide([], 'Dark', 1), -1);
});

test('keys go down and up with their modifiers, as a keyboard sends them', () => {
  const got = [];
  const env = { KeyboardEvent: function (type, init) { return { type, ...init }; } };
  S.sendKey(env, { dispatchEvent: e => got.push(e) }, 'ArrowRight', 'ArrowRight', 39, { shift: true });
  assert.deepStrictEqual(got.map(e => e.type), ['keydown', 'keyup']);
  for (const e of got) {
    assert.strictEqual(e.keyCode, 39);
    assert.ok(e.shiftKey && !e.altKey && !e.ctrlKey && e.bubbles && e.composed);
  }
});
