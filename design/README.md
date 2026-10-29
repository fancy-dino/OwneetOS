# design/

Mockups and design assets.

- [`mockups/wave1-mockups.html`](mockups/wave1-mockups.html): approved Wave 1 mockups (first
  boot, home, library, settings). Open it in a browser; it can be driven with the keyboard or a gamepad.

- [`demos/`](demos/): interactive demos, one per screen or building block, shown to the owner
  before the code is written ([[demo-before-code]]). Open them in a browser and drive them
  with a gamepad or the keyboard:
  - [`3.3-theme-foundations.html`](demos/3.3-theme-foundations.html): type scale, palette tokens
    with live contrast check, focus ring, components, text sizes, button prompts (Xbox /
    PlayStation, drawn for OwneetOS), TV safe area, 21 palettes in a picker with live preview,
    language list. Start state in the address: `#pal=ember&glyphs=ps&lang=it&ts=1.2&safe=1`
    (add `&open=palette` or `&open=language` to open a picker).
  - [`3.6-home.html`](demos/3.6-home.html): home screen (continue playing, apps, notices, recently
    played, sections on LB/RB), with the states "game running" (home opened with Guide over a
    game) and "no games yet", the options and details windows, and the placeholder sounds.
    Demo keys and start state are listed under the frame.
  - [`3.7-library.html`](demos/3.7-library.html): library (grid, filters on LT/RT, sort on Y, games
    to install, favourites, disks with free space and the Disks window).
  - [`3.9-settings.html`](demos/3.9-settings.html): settings (network and Wi-Fi password, controllers
    and pairing, sound with sliders, display with several screens, appearance, language, disks,
    system with power and credits).
  - [`5.0.0-optional-apps.html`](demos/5.0.0-optional-apps.html): the home screen's Apps grid with
    optional web apps ([[step-5.0.0-optional-web-apps]]): missing apps dimmed with a download
    mark, A installs (size, usage requirements, progress on the tile), Menu → Remove, and what can go
    wrong (live system under 12 GB of RAM, disk full, failed install; W cycles them). The engine
    behind the web apps is never named. Start state: `#installed=netflix&sim=ram&focus=apps`.
  - [`6.1-first-start.html`](demos/6.1-first-start.html): the first start
    ([[step-6.1-first-boot-flow]]): the mockup's pairing screen, the language, the network (or
    later), the time zone (suggested, or a list of cities), the Xbox adapter's firmware with
    consent (only with an adapter and the internet), ready. Start state: `#screen=time&lang=en`.
  - [`6.2-live-home.html`](demos/6.2-live-home.html) and
    [`6.2-live-settings.html`](demos/6.2-live-settings.html): started from the USB stick
    ([[step-6.2-live-vs-installed-mode]]), Home has an always visible strip above the hero,
    "Install OwneetOS", and Settings → System offers installing in place of the updates; key I
    switches live / installed. Start state: `#mode=installed`.
  - [`6.4-installer.html`](demos/6.4-installer.html): the installer ([[step-6.4-installer-ui]]):
    welcome, the internal disks with what is on them, the console's name, the warning (a switch to
    turn on for a disk with Windows or BitLocker), the final confirmation held for three seconds,
    the progress, the end and an error. Start state: `#lang=it&screen=disk`.
  - [`6.6-moving-games.html`](demos/6.6-moving-games.html): moving games between disks
    ([[step-6.6-moving-games-and-media-between-disks]]) on the games-on-disk demo, with two internal
    disks (a 96 GB SSD and a 2 TB HDD, as the FrankenPC): each cover says its disk; Menu → "Move to
    another disk" (disks without room or read only are off and say why), a confirmation with the
    switch "Remove the original after the copy is checked", then the steps ticked one by one (copy
    with Cancel, which asks first, check, the original removed; a failed check says so, with Close
    and Try again); Steam's games moved by Steam; a USB disk with games
    asks "Play from here / Copy to the console", then which disk. Start state:
    `#open=move|confirm|copying|steam|plug|plugwhere|plugrun`.
  - [`7.1-updates.html`](demos/7.1-updates.html): the updates in Settings → System
    ([[step-7.1-updates]], [[step-7.1b-going-back-to-any-version]]), on the approved settings demo:
    the automatic check's notice, "What's new" with a closed "Details" list of packages, download
    and install, "Restart now" / "Later", going back to the previous version or to any version
    (with its warning), the notice after a failed update. Start state:
    `#section=system&upd=available&lang=it` (upd: uptodate, available, ready, fellback).
  - [`7.8-website-home.html`](demos/7.8-website-home.html): the website's home
    ([[step-7.8-website]]), one page whose sections snap, the features in a carousel, the
    console's home screen drawn; English and Italian; the "Demo 7.8" button switches palette,
    light / dark and logo. Start state: `#features&lang=it&pal=dusk&logo=current`.
  - [`7.8b-website-pages.html`](demos/7.8b-website-pages.html): the website's pages
    ([[step-7.8b-website-pages]]): guide, FAQ, credits, privacy, contributing (`#/guide`, ...),
    on the approved home with the gamepad "O" logo; the logo's start-up animation (website
    variant) plays before the site. Start state: `#/faq&lang=it&pal=dusk&intro=off`.
  - [`7.8d-website-not-found.html`](demos/7.8d-website-not-found.html): the website's not-found
    page ([[step-7.8d-website-not-found-page]]), on the approved pages: shown for any address
    that leads nowhere, in Italian when the address starts with `/it/`, with the way home, the
    guide and the questions. Start state: `#/it/nowhere&pal=dusk&intro=off`.
  - [`7.8e-website-favicon.html`](demos/7.8e-website-favicon.html): the website's icon in the
    browser's tab ([[step-7.8e-website-favicon]]) from the gamepad "O": whole mark, simple mark or
    on a tile; following the device's theme or always one colour; on light and dark tab bars,
    pixel by pixel at 16 and 32 px, on a phone's home screen. Start state: `#shape=b&colour=theme`.
  - [`7.8f-website-sharing-picture.html`](demos/7.8f-website-sharing-picture.html): the picture
    shown when a link to the website is shared ([[step-7.8f-website-search-and-sharing]]):
    A the logo and the slogan (one picture for every language), B the logo and the home's title
    (one per language); Dusk or Daylight; whole and in two plain link previews. Start state:
    `#shape=b&pal=dusk&lang=it`.
  - [`logo-gamepad-o.html`](demos/logo-gamepad-o.html): a draft of a new logo, outside the
    roadmap (owner, 2026-10-19; nothing in the system uses it, the final logo is
    [[step-7.6-final-logo-and-brand]]): the first "O" as a circle with a gamepad cut out of it,
    the start-up animation (the letters come out of the "O", then the subtitle), the still boot
    splash, the top bar, small sizes and every palette. `#t=1500` in the address stops the
    animation at that time (ms).

- The interface sounds (list, length, file format), provided by the owner
  ([[step-3.12-interface-sounds]]): [[interface-sounds-spec]].

Design rules (palettes as tokens, fonts, TV safe area, input map) are in [[design-direction]],
[[design-tokens]] and [[controller-input-map]].

## Fonts

The interface fonts (Bricolage Grotesque for titles, Lexend for text, both SIL Open Font License)
are bundled in `frontend/owneet-frontend/assets/fonts/owneet/` as static instances, generated from
the pinned Google Fonts files by [`tools/branding/make-fonts`](../tools/branding/make-fonts)
(checked by SHA-256). Do not edit them by hand.

## Wordmark

The OwneetOS wordmark ("Owneet" light + "OS" coral, Bricolage Grotesque 750 / width 80, as in the
header of the mockups page; approved for the boot splash on 2026-10-05) lives in [`packages/owneet-branding/owneetos-wordmark.svg`](../packages/owneet-branding/owneetos-wordmark.svg)
as outlines (no font needed). It is generated by [`tools/branding/make-wordmark`](../tools/branding/make-wordmark);
do not edit the SVG by hand.
