// SPDX-License-Identifier: GPL-3.0-or-later
//
// Netflix's own navigation: where it applies and its names ([[extension-design]] "Netflix"); the
// rows' geometry is shared (test/shared.test.js). The parts that need Netflix's pages are checked
// in the test VM.
// Run: node --test extension/test/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const NF = require('../content/netflix.js');

test('only on Netflix', () => {
  for (const h of ['www.netflix.com', 'netflix.com', 'help.netflix.com']) assert.ok(NF.matches({ hostname: h }), h);
  for (const h of ['www.youtube.com', 'netflix.com.example.org', 'notnetflix.com']) assert.ok(!NF.matches({ hostname: h }), h);
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
