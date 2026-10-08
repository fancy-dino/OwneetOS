# frontend/

The **console interface**, `owneet-frontend`:

- [`owneet-frontend/`](owneet-frontend/): a modified version of
  [Pegasus Frontend](https://pegasus-frontend.org) by Mátyás Mustoha and contributors
  (GPL-3.0-or-later with additional terms, see `owneet-frontend/LICENSE.md`), imported unmodified
  and changed in separate commits; Qt 5.15 / QML;
- the **OwneetOS theme** in QML, inside the fork under `src/themes/`: design tokens, palettes,
  i18n message files, home, library, settings, on-screen keyboard.

As Pegasus's licence requires, the modified frontend is not called "Pegasus" and does not use
Pegasus's logos. Architecture, build, and what changes in C++ and in QML:
[`docs/frontend-architecture.md`](../docs/frontend-architecture.md). The approved look is in
[`design/mockups/`](../design/mockups/).

Roadmap: phase 3.
