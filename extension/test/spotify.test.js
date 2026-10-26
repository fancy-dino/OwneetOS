// SPDX-License-Identifier: GPL-3.0-or-later
//
// Spotify's own navigation in its player for an Android TV: where it applies, its names, the items
// found by their label, the tabs on LB / RB, the seeking of LT / RT ([[extension-design]]
// "Spotify"). The parts that need Spotify's pages are checked in the test VM.
// Run: node --test extension/test/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const SP = require('../content/spotify.js');

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
  assert.match(SP.NAMES.card, /home-card/);
  assert.match(SP.NAMES.card, /card-mwp/);
});

test('X finds "Add to queue" by its label, in English and Italian', () => {
  for (const l of ['Add to queue', 'add to queue', 'Aggiungi in coda', 'Aggiungi alla coda']) assert.ok(SP.isQueue(l), l);
  for (const l of ['Add to playlist', 'Add to Liked Songs', 'Go to queue', 'Remove from queue', '']) assert.ok(!SP.isQueue(l), l);
});

test('the items that open what the console cannot show are never reached', () => {
  for (const l of ['Open App', 'Get App', 'Open the app', "Apri l'app", "Scarica l'app", 'Open in Desktop app']) {
    assert.ok(SP.isNeverLabel(l), l);
  }
  for (const l of ['Add to queue', 'Go to album', 'Share', 'App music mix', 'Happy']) assert.ok(!SP.isNeverLabel(l), l);
});

test("B closes Spotify's windows by their Close / Cancel button", () => {
  for (const l of ['Close', 'Cancel', 'Chiudi', 'Annulla']) assert.ok(SP.isClose(l), l);
  for (const l of ['Closer', 'Close Friends', '']) assert.ok(!SP.isClose(l), l);
});

test('LB / RB: Home, Search, Your Library, going round', () => {
  assert.strictEqual(SP.tabAfter('home', 1), 'search');
  assert.strictEqual(SP.tabAfter('search', 1), 'library');
  assert.strictEqual(SP.tabAfter('library', 1), 'home');
  assert.strictEqual(SP.tabAfter('home', -1), 'library');
  assert.strictEqual(SP.tabAfter('search', -1), 'home');
});

test("LT / RT: Spotify's time labels and where the bar is clicked", () => {
  assert.strictEqual(SP.seconds('0:35'), 35);
  assert.strictEqual(SP.seconds('3:10'), 190);
  assert.strictEqual(SP.seconds('1:02:03'), 3723);
  assert.ok(Number.isNaN(SP.seconds('-2:35')));
  assert.ok(Number.isNaN(SP.seconds('Live')));
  assert.strictEqual(SP.seekFraction(95, 190, 5), 0.5263157894736842);
  assert.strictEqual(SP.seekFraction(2, 190, -5), 0);           // not before the start
  assert.strictEqual(SP.seekFraction(188, 190, 5), 1);          // nor past the end
  assert.ok(Number.isNaN(SP.seekFraction(10, 0, 5)));           // no length: no click
});

test('on a TV Spotify lays out as on a screen 1280 px wide', () => {
  assert.strictEqual(SP.LAYOUT_WIDTH, 1280);
});
