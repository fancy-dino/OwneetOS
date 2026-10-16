// SPDX-License-Identifier: GPL-3.0-or-later
//
// The geometry of the D-pad's spatial navigation ([[extension-design]]).
// Run: node --test extension/test/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const Nav = require('../content/spatnav.js');

const box = (left, top, width, height) => ({ left, top, width, height, right: left + width, bottom: top + height });
// level 0: in the current element's own box; higher: larger boxes
const cand = (rect, level = 0, sameRow = true) => ({ rect, level, sameRow });

// The owner's example (2026-10-16):
//   el1, el2 (whole width) / <box> el3 | el4 </box> / el5 (whole width)
const el1 = box(0, 0, 1000, 40), el2 = box(0, 50, 1000, 40), el3 = box(0, 100, 300, 40),
  el4 = box(320, 100, 680, 40), el5 = box(0, 150, 1000, 40);

test('the box first: right from el3 is el4, and back', () => {
  assert.strictEqual(Nav.pick(el3, [cand(el1, 1), cand(el2, 1), cand(el4, 0), cand(el5, 1)], 'right'), 2);
  assert.strictEqual(Nav.pick(el4, [cand(el1, 1), cand(el2, 1), cand(el3, 0), cand(el5, 1)], 'left'), 2);
});

test('then outward: up from el4 is el2, down from el3 is el5', () => {
  assert.strictEqual(Nav.pick(el4, [cand(el1, 1), cand(el2, 1), cand(el3, 0), cand(el5, 1)], 'up'), 1);
  assert.strictEqual(Nav.pick(el3, [cand(el1, 1), cand(el2, 1), cand(el4, 0), cand(el5, 1)], 'down'), 3);
});

test('a farther element in its own box wins over a nearer one outside it', () => {
  const a = box(0, 0, 100, 40);
  assert.strictEqual(Nav.pick(a, [cand(box(150, 0, 100, 40), 1), cand(box(600, 0, 100, 40), 0)], 'right'), 1);
});

test('in line wins over diagonal, at any level', () => {
  const c = box(400, 0, 100, 50);
  // diagonal in the same box, nearer; in line outside the box, farther
  assert.strictEqual(Nav.pick(c, [cand(box(0, 70, 100, 50), 0), cand(box(420, 300, 100, 50), 2)], 'down'), 1);
});

test('diagonal when nothing is in line (header to hero)', () => {
  assert.strictEqual(Nav.pick(box(1100, 20, 80, 30), [cand(box(60, 300, 120, 40), 3)], 'down'), 0);
});

test('of two in line, the better aligned wins', () => {
  const c = box(200, 0, 200, 50);
  assert.strictEqual(Nav.pick(c, [cand(box(0, 80, 250, 50)), cand(box(250, 80, 250, 50))], 'down'), 1);
});

test('the column is remembered across a wide element', () => {
  const card = box(600, 0, 200, 100), banner = box(0, 150, 1280, 200);
  const row = [cand(box(0, 400, 200, 100)), cand(box(300, 400, 200, 100)), cand(box(600, 400, 200, 100))];
  // without memory every card lines up with the banner: the first one wins
  assert.strictEqual(Nav.pick(banner, row, 'down'), 0);
  // card → banner set the column at the card's centre; banner → the card below it
  const anchor = Nav.anchorAfter(null, card, 'down');
  assert.strictEqual(Nav.pick(banner, row, 'down', anchor), 2);
  // moving sideways forgets it
  assert.notStrictEqual(Nav.anchorAfter(anchor, banner, 'right').axis, 'v');
});

test('off-screen elements count', () => {
  assert.strictEqual(Nav.pick(box(1000, 400, 200, 120), [cand(box(1300, 400, 200, 120))], 'right'), 0);
});

test('nothing in that direction', () => {
  assert.strictEqual(Nav.pick(box(0, 0, 100, 50), [cand(box(0, 100, 100, 50))], 'up'), -1);
});

test('reading order: line by line, then left to right', () => {
  const nav = box(200, 26, 60, 20), search = box(1000, 20, 34, 34), play = box(60, 300, 120, 40);
  assert.deepStrictEqual([play, search, nav].sort(Nav.readingOrder), [nav, search, play]);
});

// ---------- Left / right: the container, then the target before / after it (pickSideways) ----------

// The demo's home page (owner's screenshot, 2026-10-16): Play, More info / a row of cards
const play = box(155, 285, 210, 95), more = box(395, 285, 265, 95);
const card1 = box(63, 620, 490, 360), card2 = box(582, 620, 490, 360), card3 = box(1101, 620, 490, 360);
const home = [play, more, card1, card2, card3];

test('left from the second card is the first card, not the nearer "More info"', () => {
  assert.strictEqual(Nav.pickSideways(card2, home, 3, 'left', 2, 4), 2);
});

test('left from the first child is the target before it (before the container), wherever it is', () => {
  assert.strictEqual(Nav.pickSideways(card1, home, 2, 'left', 2, 4), 1);
  assert.strictEqual(Nav.pickSideways(play, home, 0, 'left', 0, 1), -1);
});

test('right from the last child is the target after it (after the container)', () => {
  assert.strictEqual(Nav.pickSideways(more, home, 1, 'right', 0, 1), 2);
  assert.strictEqual(Nav.pickSideways(card3, home, 4, 'right', 2, 4), -1);
});

test('the owner\'s example, sideways: <box> el3 el4 </box>', () => {
  const order = [el1, el2, el3, el4, el5];
  assert.strictEqual(Nav.pickSideways(el3, order, 2, 'right', 2, 3), 3);
  assert.strictEqual(Nav.pickSideways(el4, order, 3, 'left', 2, 3), 2);
  assert.strictEqual(Nav.pickSideways(el3, order, 2, 'left', 2, 3), 1);     // first child: el2, before the box
});

test('a full-width banner in a large container: left / right go to the targets before / after it', () => {
  // Layouts page of the demo: row of tiles, banner, row of tiles, all in one container
  const above = box(56, 100, 170, 70), banner = box(56, 180, 1100, 80), below = box(56, 270, 170, 70);
  const order = [box(0, 0, 100, 40), above, banner, below, box(0, 400, 100, 40)];
  assert.strictEqual(Nav.pickSideways(banner, order, 2, 'left', 0, 4), 1);
  assert.strictEqual(Nav.pickSideways(banner, order, 2, 'right', 0, 4), 3);
});
