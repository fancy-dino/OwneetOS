// SPDX-License-Identifier: GPL-3.0-or-later
//
// Spotify's own navigation: where it applies, its names, the items found by their label, the
// sections on LB / RB ([[extension-design]] "Spotify"). The parts that need Spotify's pages are
// checked in the test VM.
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

test('on a TV Spotify lays out as on a screen 1280 px wide', () => {
  assert.strictEqual(SP.LAYOUT_WIDTH, 1280);
});
