# extension/

**Brave extension** (Manifest V3) that makes streaming sites usable with a gamepad:

- Gamepad API loop and spatial navigation;
- B goes back, Guide returns to home (handled by the system daemon);
- per-site scripts (YouTube TV interface, Netflix, Spotify);
- on-screen keyboard for web forms (the console's own keyboard).

Files:

- `content/pad.js`: the controller read in the standard layout, also a controller Brave does not
  know (Xbox-style controllers on the xpad driver are translated; tested by `test/pad.test.js`);
- `content/spatnav.js`: which elements the D-pad reaches and which one comes next (the geometry is
  tested by `test/spatnav.test.js`, run by `tools/lint`);
- `content/input.js`: the Gamepad API loop, the ring, A, B, the right stick, text fields;
- `content/youtube-tv.js`: on YouTube's TV interface the buttons become its keys (arrows, Enter,
  Escape) and B asks to close where Escape changes nothing (tested by `test/youtube-tv.test.js`);
- `content/shared.js`: what the supported apps' navigations share (rows of tiles that slide on,
  keys sent as a keyboard sends them; tested by `test/shared.test.js`);
- `content/netflix.js`: Netflix's own navigation (rows and tiles, the title window, the player's
  controls, LT / RT ±10 s, Y search), Netflix's element names in one place; elsewhere the generic
  navigation (tested by `test/netflix.test.js`);
- `content/spotify.js`: Spotify's own navigation (shelves and cards, song lists, Your Library, X
  queue, Y the player bar, Menu its context menu, LT / RT ±5 s, LB / RB Home / Search / Your
  Library), drawn bigger on a TV and its right panel folded; Spotify's element names in one place
  (tested by `test/spotify.test.js`);
- `background.js`: relays the page's messages to `owneet-bridge`, the native-messaging host that
  talks to owneetd (the console's keyboard, "Close {name}?"), and sets Brave's zoom for an app
  drawn bigger on a TV (Spotify). Installed as
  `background-<hash of its content>.js`, and the installed `manifest.json` gets that name and
  owneet-base's version (`<pkgver>.<pkgrel>`): a used profile picks up a changed worker only when
  both change ([[brave-extension-worker-cached]]). The `version` in this folder is not shipped.

Shipped by `owneet-base` in `/usr/share/owneet/extension/` and loaded by owneetd when it opens a
web app (`--load-extension`): no store and no update server. No third-party code or logos.
Design: [[extension-design]].

Plan: [[phase-5-streaming-apps-and-media]].
