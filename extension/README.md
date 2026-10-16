# extension/

**Brave extension** (Manifest V3) that makes streaming sites usable with a gamepad:

- Gamepad API loop and spatial navigation;
- B goes back, Guide returns to home (handled by the system daemon);
- per-site scripts (YouTube TV interface, Netflix, Spotify);
- on-screen keyboard for web forms (the console's own keyboard).

Files:

- `content/spatnav.js`: which elements the D-pad reaches and which one comes next (the geometry is
  tested by `test/spatnav.test.js`, run by `tools/lint`);
- `content/input.js`: the Gamepad API loop, the ring, A, B, the right stick, text fields;
- `background.js`: relays the page's messages to `owneet-bridge`, the native-messaging host that
  talks to owneetd (the console's keyboard, "Close {name}?"). Installed as
  `background-<hash of its content>.js`, and the installed `manifest.json` gets that name and
  owneet-base's version (`<pkgver>.<pkgrel>`): a used profile picks up a changed worker only when
  both change ([[brave-extension-worker-cached]]). The `version` in this folder is not shipped.

Shipped by `owneet-base` in `/usr/share/owneet/extension/` and loaded by owneetd when it opens a
web app (`--load-extension`): no store and no update server. No third-party code or logos.
Design: [[extension-design]].

Plan: [[phase-5-streaming-apps-and-media]].
