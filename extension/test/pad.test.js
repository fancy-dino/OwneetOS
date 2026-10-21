// SPDX-License-Identifier: GPL-3.0-or-later
//
// The controller read in the standard layout ([[extension-design]] "Input"), also when Brave does
// not know it (e.g. the Nacon GC-100 of the test VM: 11 buttons, 8 axes, mapping "").
// Run: node --test extension/test/*.test.js

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const Pad = require('../content/pad.js');

// A controller as Brave reports it in the xpad driver's order
function xpad({ buttons = [], axes = {} } = {}) {
  const a = [0, 0, -1, 0, 0, -1, 0, 0];             // triggers released: −1
  for (const [i, v] of Object.entries(axes)) a[i] = v;
  return { id: 'Nacon GC-100 (Vendor: 3285 Product: 0607)', mapping: '',
    buttons: Array.from({ length: 11 }, (_, i) => ({ pressed: buttons.includes(i), value: buttons.includes(i) ? 1 : 0 })),
    axes: a };
}
const pressed = p => p.buttons.map((b, i) => (b.pressed ? i : -1)).filter(i => i >= 0);

test('a standard controller is read as it comes', () => {
  const p = { mapping: 'standard', buttons: [], axes: [] };
  assert.strictEqual(Pad.standard(p), p);
  assert.strictEqual(Pad.standard(null), null);
});

test('xpad order: the face and shoulder buttons, View and Menu, the sticks pressed', () => {
  const p = Pad.standard(xpad({ buttons: [0, 1, 2, 3, 4, 5, 6, 7, 9, 10] }));
  assert.strictEqual(p.mapping, 'standard');
  assert.strictEqual(p.buttons.length, Pad.BUTTONS);
  assert.deepStrictEqual(pressed(p), [0, 1, 2, 3, 4, 5, 8, 9, 10, 11]);
});

test('Guide (button 8 there) is never passed on', () => {
  assert.deepStrictEqual(pressed(Pad.standard(xpad({ buttons: [8] }))), []);
});

test('the D-pad on axes 6 / 7 becomes buttons 12–15', () => {
  assert.deepStrictEqual(pressed(Pad.standard(xpad({ axes: { 7: -1 } }))), [12]);   // up
  assert.deepStrictEqual(pressed(Pad.standard(xpad({ axes: { 7: 1 } }))), [13]);    // down
  assert.deepStrictEqual(pressed(Pad.standard(xpad({ axes: { 6: -1 } }))), [14]);   // left
  assert.deepStrictEqual(pressed(Pad.standard(xpad({ axes: { 6: 1 } }))), [15]);    // right
});

test('the triggers on axes 2 / 5 become buttons 6 / 7 with their travel', () => {
  const p = Pad.standard(xpad({ axes: { 2: 1, 5: 0 } }));    // LT pressed, RT half way
  assert.deepStrictEqual(pressed(p), [6]);
  assert.strictEqual(p.buttons[6].value, 1);
  assert.strictEqual(p.buttons[7].value, 0.5);
  // released (−1); before their first movement Linux reports 0: half way, not pressed
  const rest = Pad.standard(xpad({ axes: { 2: -1, 5: 0 } }));
  assert.deepStrictEqual(pressed(rest), []);
  assert.strictEqual(rest.buttons[6].value, 0);
});

test('the sticks: left on axes 0 / 1, right on axes 3 / 4 (not the triggers)', () => {
  const p = Pad.standard(xpad({ axes: { 0: 0.9, 1: -0.8, 2: 1, 3: -0.7, 4: 0.6 } }));
  assert.deepStrictEqual(p.axes, [0.9, -0.8, -0.7, 0.6]);
});

test('other layouts are read as they come', () => {
  const p = { mapping: '', buttons: new Array(13).fill({ pressed: false, value: 0 }), axes: [0, 0, 0, 0] };
  assert.strictEqual(Pad.standard(p), p);
});
