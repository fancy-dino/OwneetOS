// SPDX-License-Identifier: GPL-3.0-or-later
//
// Spotify's own navigation in its desktop web player: where it applies, its names, the items found
// by their label, the sections on LB / RB, and its D-pad, which moves by position only
// ([[extension-design]] "Spotify"). The parts that need Spotify's pages are checked in the test VM.
// Run: node --test extension/test/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const SP = require('../content/spotify.js');

// A box: left, top, width, height
const b = (left, top, w = 100, h = 100) => ({ left, top, right: left + w, bottom: top + h });

test('only on the web player', () => {
  assert.ok(SP.matches({ hostname: 'open.spotify.com' }));
  for (const h of ['www.spotify.com', 'accounts.spotify.com', 'open.spotify.com.example.org', 'spotify.com']) {
    assert.ok(!SP.matches({ hostname: h }), h);
  }
});

test("Spotify's names are valid selectors in one place", () => {
  for (const [name, sel] of Object.entries(SP.NAMES)) {
    assert.ok(typeof sel === 'string' && sel.length > 0, name);
    // balanced brackets and quotes: a typo would break the whole query
    assert.strictEqual((sel.match(/\[/g) || []).length, (sel.match(/\]/g) || []).length, name);
    assert.strictEqual((sel.match(/"/g) || []).length % 2, 0, name);
  }
  assert.match(SP.NAMES.never, /pip-toggle-button/);
});

test('X finds "Add to queue" by its label, in English and Italian', () => {
  for (const l of ['Add to queue', 'add to queue', 'Aggiungi in coda', 'Aggiungi alla coda']) assert.ok(SP.isQueue(l), l);
  for (const l of ['Add to playlist', 'Add to Liked Songs', 'Go to queue', 'Remove from queue', '']) assert.ok(!SP.isQueue(l), l);
});

test('the items that open what the console cannot show are never reached', () => {
  for (const l of ['Open in Desktop app', "Apri nell'app desktop", 'Install App', "Installa l'app", 'Open Miniplayer']) {
    assert.ok(SP.isNeverLabel(l), l);
  }
  for (const l of ['Add to queue', 'Go to album', 'Share', 'Desktop music for work']) assert.ok(!SP.isNeverLabel(l), l);
});

test('LB / RB: Home, Search, Your Library, going round', () => {
  assert.strictEqual(SP.sectionAfter('home', 1), 'search');
  assert.strictEqual(SP.sectionAfter('search', 1), 'library');
  assert.strictEqual(SP.sectionAfter('library', 1), 'home');
  assert.strictEqual(SP.sectionAfter('home', -1), 'library');
  assert.strictEqual(SP.sectionAfter('search', -1), 'home');
});

test('the D-pad: only boxes past the edge in the direction count', () => {
  const c = b(200, 200);
  const around = [b(0, 200), b(400, 200), b(200, 0), b(200, 400)];   // left, right, up, down
  assert.strictEqual(SP.spatialPick(c, around, 'left'), 0);
  assert.strictEqual(SP.spatialPick(c, around, 'right'), 1);
  assert.strictEqual(SP.spatialPick(c, around, 'up'), 2);
  assert.strictEqual(SP.spatialPick(c, around, 'down'), 3);
  assert.strictEqual(SP.spatialPick(c, [b(400, 200)], 'left'), -1);
  assert.strictEqual(SP.spatialPick(c, [], 'down'), -1);
  // overlapping by up to 4 px still counts as past; more does not
  assert.strictEqual(SP.spatialPick(c, [b(297, 200)], 'right'), 0);
  assert.strictEqual(SP.spatialPick(c, [b(290, 200)], 'right'), -1);
});

test('the D-pad: a box lined up across wins over a nearer one aside', () => {
  const c = b(0, 0);
  // right: lined up 100 px away, aside only 10 px away but 50 px lower
  assert.strictEqual(SP.spatialPick(c, [b(110, 150), b(200, 0)], 'right'), 1);
  // down: the card below beats the nearer one down and to the right
  assert.strictEqual(SP.spatialPick(c, [b(150, 110), b(0, 300)], 'down'), 1);
});

test('the D-pad: among lined-up boxes the nearest; among the others gap + twice the offset', () => {
  const c = b(0, 0);
  assert.strictEqual(SP.spatialPick(c, [b(400, 0), b(150, 20)], 'right'), 1);
  // none lined up: 200 along + 2 × 50 across = 300 beats 50 along + 2 × 300 across = 650
  assert.strictEqual(SP.spatialPick(c, [b(150, 400), b(300, 150)], 'right'), 1);
  // a tie: the first in the page's order
  assert.strictEqual(SP.spatialPick(c, [b(0, 150), b(0, 150)], 'down'), 0);
});

test('on a TV Spotify lays out as on a screen 1280 px wide', () => {
  assert.strictEqual(SP.LAYOUT_WIDTH, 1280);
});
