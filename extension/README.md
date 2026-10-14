# extension/

**Brave extension** (Manifest V3) that makes streaming sites usable with a gamepad:

- Gamepad API loop and spatial navigation;
- B goes back, Guide returns to home (handled by the system daemon);
- per-site scripts (YouTube TV interface, Netflix, Spotify);
- on-screen keyboard for web forms.

Shipped by `owneet-base` in `/usr/share/owneet/extension/` and loaded by owneetd when it opens a
web app (`--load-extension`): no store and no update server. Since step 5.1 it is an empty
manifest that only proves the loading; its features come in step 5.2. No third-party logos are
bundled.

Plan: [[phase-5-streaming-apps-and-media]].
