// SPDX-License-Identifier: GPL-3.0-or-later
//
// The geometry of the D-pad's spatial navigation ([[extension-design]]). Run: node --test extension/test/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const Nav = require('../content/spatnav.js');

const box = (left, top, width, height) => ({ left, top, width, height, right: left + width, bottom: top + height });
const cand = (rect, sameRow = true) => ({ rect, sameRow });

test('right picks the nearest element in the same line', () => {
  const c = box(0, 0, 100, 50);
  const list = [cand(box(300, 0, 100, 50)), cand(box(120, 0, 100, 50)), cand(box(-200, 0, 100, 50))];
  assert.strictEqual(Nav.choose(c, list, 'right'), 1);
});

test('down prefers an element overlapping across over a nearer diagonal one', () => {
  const c = box(400, 0, 100, 50);
  const list = [cand(box(0, 70, 100, 50)), cand(box(420, 200, 100, 50))];
  assert.strictEqual(Nav.choose(c, list, 'down'), 1);
});

test('down reaches an element far to the side when nothing overlaps (header to hero)', () => {
  const c = box(1100, 20, 80, 30);
  assert.strictEqual(Nav.choose(c, [cand(box(60, 300, 120, 40))], 'down'), 0);
});

test('off-screen elements count', () => {
  const c = box(1000, 400, 200, 120);
  assert.strictEqual(Nav.choose(c, [cand(box(1300, 400, 200, 120))], 'right'), 0);
});

test('the end of a row does not jump sideways into another row', () => {
  const c = box(1000, 400, 200, 120);
  const otherRow = cand(box(1300, 560, 200, 120), false);
  assert.strictEqual(Nav.choose(c, [otherRow], 'right'), -1);
});

test('nothing in that direction', () => {
  assert.strictEqual(Nav.choose(box(0, 0, 100, 50), [cand(box(0, 100, 100, 50))], 'up'), -1);
});

test('reading order: line by line, then left to right', () => {
  const nav = box(200, 26, 60, 20), search = box(1000, 20, 34, 34), play = box(60, 300, 120, 40);
  const sorted = [play, search, nav].sort(Nav.readingOrder);
  assert.deepStrictEqual(sorted, [nav, search, play]);
});
