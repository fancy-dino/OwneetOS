// SPDX-License-Identifier: GPL-3.0-or-later
//
// The controller read in the standard layout of the Gamepad API ([[extension-design]] "Input"),
// whatever Brave reports. Brave knows the layout of well-known controllers ("standard"); others
// come in the Linux driver's order. The common one, Xbox-style controllers on the xpad driver
// (11 buttons, 8 axes: e.g. the Nacon GC-100 in the test VM, 2026-10-22), is translated: buttons
// A B X Y LB RB View Menu Guide LS RS, axes left stick, LT (−1 released … +1 pressed), right stick,
// RT, and the D-pad on axes 6 / 7. Other layouts are read as they come. Guide is never passed on
// ([[guide-button]]).
// Tests: test/pad.test.js.

'use strict';

var OwneetPad = (() => {
  const BUTTONS = 17;                      // the standard layout: 0–15, and 16 (Guide, never read)
  const button = (pressed, value) => ({ pressed, value });

  // A trigger on an axis (−1 released … +1 pressed) as a button's value (0 … 1). Linux reports 0
  // until the trigger first moves: half way, which counts as released (pressed: past half)
  const triggerValue = v => (v + 1) / 2;
  // One side of the D-pad on an axis (−1 / +1) as a button
  const dpad = (v, side) => { const on = side < 0 ? v < -0.5 : v > 0.5; return button(on, on ? 1 : 0); };

  function xpad(p) {
    const b = i => p.buttons[i] || button(false, 0);
    const a = i => p.axes[i] || 0;
    const fromAxis = v => button(v > 0.5, v);
    const out = [
      b(0), b(1), b(2), b(3),                        // A B X Y
      b(4), b(5),                                    // LB RB
      fromAxis(triggerValue(a(2))), fromAxis(triggerValue(a(5))),   // LT RT
      b(6), b(7),                                    // View Menu
      b(9), b(10),                                   // LS RS
      dpad(a(7), -1), dpad(a(7), 1),                 // up down
      dpad(a(6), -1), dpad(a(6), 1),                 // left right
      button(false, 0),                              // Guide: owneetd's
    ];
    return { id: p.id, mapping: 'standard', buttons: out, axes: [a(0), a(1), a(3), a(4)] };
  }

  // The controller in the standard layout (as it comes when Brave knows it, or nothing to translate)
  function standard(p) {
    if (!p || p.mapping === 'standard') return p;
    if (p.buttons.length === 11 && p.axes.length === 8) return xpad(p);
    return p;
  }

  return { standard, BUTTONS };
})();

if (typeof module !== 'undefined') module.exports = OwneetPad;
