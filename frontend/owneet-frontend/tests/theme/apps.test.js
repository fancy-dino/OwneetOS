// SPDX-License-Identifier: GPL-3.0-or-later
//
// The order of the apps on the home screen and on the Apps page ([[step-5.7-apps-page]], approved
// demo design/demos/5.7-apps-page.html): the theme's foundation/apps.js.
// Run: node --test frontend/owneet-frontend/tests/theme/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const Apps = require('../../src/themes/owneet/foundation/apps.js');

const web = (...list) => list.map(s => ({ id: s.replace('-', ''), installed: !s.startsWith('-') }));

test('the fixed order: the built-in apps, then the web apps in the catalogue\'s order', () => {
  const apps = Apps.fixedOrder(web('netflix', '-youtube', 'spotify'));
  assert.deepStrictEqual(apps.map(a => a.key), ['videos', 'music', 'store', 'web:netflix', 'web:youtube', 'web:spotify']);
  assert.deepStrictEqual(apps.map(a => a.installed), [true, true, true, true, false, true]);
});

test('six apps or fewer: every app on the home, a missing one in its place', () => {
  const apps = Apps.fixedOrder(web('netflix', '-youtube', 'spotify'));
  assert.deepStrictEqual(Apps.homeOrder(apps, { 'web:spotify': 40 }),
    ['videos', 'music', 'store', 'web:netflix', 'web:youtube', 'web:spotify']);
});

test('the Apps page: the installed apps before the missing ones', () => {
  const apps = Apps.fixedOrder(web('netflix', '-youtube', 'spotify', 'filmclub', 'podcasts', '-radio'));
  assert.deepStrictEqual(Apps.pageOrder(apps),
    ['videos', 'music', 'store', 'web:netflix', 'web:spotify', 'web:filmclub', 'web:podcasts', 'web:youtube', 'web:radio']);
});

test('more than six apps, six or fewer installed: the first five of the page and "All apps"', () => {
  // six installed, one missing: the sixth installed only on the page (owner, 2026-11-20)
  const apps = Apps.fixedOrder(web('netflix', 'spotify', 'filmclub', '-youtube'));
  assert.deepStrictEqual(Apps.homeOrder(apps, { 'web:filmclub': 99 }),
    ['videos', 'music', 'store', 'web:netflix', 'web:spotify', Apps.ALL_APPS]);
  // a missing app before an installed one in the catalogue: after them
  const few = Apps.fixedOrder(web('-netflix', 'spotify', '-youtube', '-radio'));
  assert.deepStrictEqual(Apps.homeOrder(few, {}), ['videos', 'music', 'store', 'web:spotify', 'web:netflix', Apps.ALL_APPS]);
});

test('from the seventh installed app: the five most used, equal counts in the fixed order', () => {
  const apps = Apps.fixedOrder(web('netflix', 'spotify', 'filmclub', 'podcasts', '-youtube', '-radio'));
  const uses = { videos: 3, music: 12, store: 5, 'web:netflix': 20, 'web:spotify': 30, 'web:filmclub': 8, 'web:podcasts': 1 };
  assert.deepStrictEqual(Apps.homeOrder(apps, uses),
    ['web:spotify', 'web:netflix', 'music', 'web:filmclub', 'store', Apps.ALL_APPS]);
  // the store as used as Music: after it (the fixed order)
  assert.deepStrictEqual(Apps.homeOrder(apps, { ...uses, store: 12 }).slice(0, 4), ['web:spotify', 'web:netflix', 'music', 'store']);
  // nothing used yet: the fixed order
  assert.deepStrictEqual(Apps.homeOrder(apps, {}), ['videos', 'music', 'store', 'web:netflix', 'web:spotify', Apps.ALL_APPS]);
  // a missing app never takes a place, whatever its old count
  assert.ok(!Apps.homeOrder(apps, { 'web:youtube': 500 }).includes('web:youtube'));
});

test('removing an app clears its count', () => {
  const apps = Apps.fixedOrder(web('netflix', '-youtube'));
  assert.deepStrictEqual(Apps.pruneUses(apps, { music: 2, 'web:netflix': 4, 'web:youtube': 9, 'web:gone': 1 }),
    { music: 2, 'web:netflix': 4 });
  assert.strictEqual(Apps.pruneUses(apps, { music: 2 }), null);      // nothing to change
});
