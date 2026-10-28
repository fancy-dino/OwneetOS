// SPDX-License-Identifier: GPL-3.0-or-later
//
// What the navigations of the supported web apps share ([[extension-design]] "One navigation per
// supported app"): rows of tiles that slide on (Netflix's rows) and keys sent as a keyboard sends
// them (Netflix, Spotify). Tested without a browser (test/shared.test.js).

'use strict';

var OwneetShared = (() => {
  // The index of the box whose centre is nearest to x across (-1: none)
  function nearestTo(x, rects) {
    let best = -1, bestD = Infinity;
    rects.forEach((r, i) => {
      const d = Math.abs(r.left + r.width / 2 - x);
      if (d < bestD) { best = i; bestD = d; }
    });
    return best;
  }
  // Is the box fully inside the area across (left / right; 2 px of slack)?
  const insideAcross = (r, area) => r.left >= area.left - 2 && r.right <= area.right + 2;
  // After a row slid on: the tile to select among the visible ones (rects), given the label of the
  // tile that was wanted (labels) and the direction (1: right, -1: left)
  function afterSlide(labels, wanted, step) {
    const same = wanted ? labels.indexOf(wanted) : -1;
    if (same >= 0) return same;
    return labels.length ? (step > 0 ? 0 : labels.length - 1) : -1;
  }
  // keydown and keyup on target, as a keyboard sends them; mods: { shift, alt, ctrl }
  function sendKey(env, target, key, code, keyCode, mods = {}) {
    for (const type of ['keydown', 'keyup']) {
      target.dispatchEvent(new env.KeyboardEvent(type, { key, code, keyCode, which: keyCode,
        shiftKey: !!mods.shift, altKey: !!mods.alt, ctrlKey: !!mods.ctrl,
        bubbles: true, cancelable: true, composed: true }));
    }
  }

  return { nearestTo, insideAcross, afterSlide, sendKey };
})();

if (typeof module !== 'undefined') module.exports = OwneetShared;
