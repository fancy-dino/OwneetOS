// SPDX-License-Identifier: GPL-3.0-or-later
// The command legends ([[step-5.6a-command-legend]], approved demo
// design/demos/5.6a-command-legend.html, [[controller-input-map]]): what each button does in an
// app, for the screen on show, as rows of [buttons, message]; a row without buttons is a line of
// its own. The app tells the console which legend to show (owneetd's "legend.changed", context):
// "web" (a web app without a navigation of its own), "youtube", "netflix.browse", "netflix.title",
// "netflix.player", "spotify", "player.video", "player.song".
// One source: the interface draws them (LegendView.qml), and tests check them against the
// bindings: extension/test/legend.test.js (the web apps' navigations) and
// daemon/internal/player/bindings_test.go (the player's input.conf). Keep the object strict JSON
// (the Go test reads it); its texts are in the message files (legend.*). View (close) and Guide
// close every legend: the interface adds them.
var LEGENDS =
{
  "web": [
    [["dpad", "ls"], "legend.web.move"], [["a"], "legend.web.a"], [["b"], "legend.web.b"], [["rs"], "legend.rs"]
  ],
  "youtube": [
    [["dpad", "ls"], "legend.yt.move"], [["a"], "legend.yt.a"], [["b"], "legend.yt.b"]
  ],
  "netflix.browse": [
    [["dpad", "ls"], "legend.nf.browse.move"], [["a"], "legend.nf.browse.a"], [["y"], "legend.nf.browse.y"],
    [["rs"], "legend.rs"], [["b"], "legend.nf.browse.b"]
  ],
  "netflix.title": [
    [["dpad", "ls"], "legend.nf.title.move"], [["a"], "legend.nf.title.a"], [["b"], "legend.nf.title.b"]
  ],
  "netflix.player": [
    [[], "legend.nf.player.any"], [["dpad", "ls"], "legend.nf.player.move"], [["a"], "legend.nf.player.a"],
    [["lt", "rt"], "legend.seek10"], [["b"], "legend.nf.player.b"]
  ],
  "spotify": [
    [["dpad", "ls"], "legend.sp.move"], [["a"], "legend.sp.a"], [["x"], "legend.sp.x"], [["y"], "legend.sp.y"],
    [["menu"], "legend.sp.menu"], [["lt", "rt"], "legend.seek5"], [["r3"], "legend.sp.r3"], [["rs"], "legend.rs"],
    [["b"], "legend.sp.b"]
  ],
  "player.video": [
    [["a"], "legend.pl.a"], [["dpad"], "legend.pl.seek"], [["lt", "rt"], "legend.pl.video"], [["x"], "legend.pl.audio"],
    [["menu"], "legend.pl.subs"], [["y"], "legend.pl.autoplay"], [["b"], "legend.pl.b"]
  ],
  "player.song": [
    [["a"], "legend.pl.a"], [["dpad"], "legend.pl.seek"], [["lt", "rt"], "legend.pl.song"], [["b"], "legend.pl.b"]
  ]
}
;
if (typeof module !== "undefined") module.exports = LEGENDS;
