# Contributing to OwneetOS

Read [PROJECT_RULES.md](PROJECT_RULES.md) first. It is the source of truth for requirements and
decisions, and nothing in this file overrides it.

## Language

**English only** for code, comments, documentation, commit messages, issues and pull requests.
The user interface is multilingual through message files, never through hard-coded text.

## Workflow

1. Work follows [ROADMAP.md](ROADMAP.md), **one step at a time, in order**.
2. Each step ends with a short report: what was done, how it was verified, what deviated from the
   plan. The project owner approves the step before the next one starts.
3. A change of plan is recorded in the PROJECT_RULES.md decision log and reflected in the roadmap
   in the same change.
4. Anything that installs software on a developer's machine, or creates remote resources, needs the
   owner's explicit approval at that moment.

## Development environment

- Never build or test on the host system. ISOs and packages are built in the **builder VM** and
  booted in the **test VM** (see [`vm/`](vm/)).
- Everything stays inside the repository folder. VM images, caches and build output are ignored by
  git (see `.gitignore`).

## Checks

Run every check before committing. The tools live in the builder VM, not on the host:

```text
tools/build-in-vm tools/lint
```

It runs shellcheck on every shell script, verifies the SPDX license headers and runs markdownlint
on every Markdown file. CI runs the same script on every push and pull request
([`.github/workflows/ci.yml`](.github/workflows/ci.yml)).

## Branches and commits

- `main` must always build. Work happens on short-lived branches named
  `<area>/<short-description>`, for example `daemon/bluetooth-autopair` or `iso/plymouth-theme`.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):
  `type(scope): summary`, for example `feat(daemon): auto-pair gamepads over Bluetooth`.
  - Types: `feat`, `fix`, `docs`, `refactor`, `test`, `build`, `ci`, `chore`.
  - Scopes match the top-level folders: `iso`, `packages`, `daemon`, `frontend`, `extension`,
    `installer`, `tools`, `vm`, `docs`, `design`.
- Keep commits focused: one logical change per commit.

## Rules that apply to every component

- **Lightweight first.** Every new dependency must be justified by what it saves us; measure RAM and
  disk impact when in doubt.
- **Gamepad only.** Every feature must be fully usable with a controller. Follow the global input
  map in PROJECT_RULES.md section 9.1; never bind the Guide button outside the system daemon.
- **No hard-coded UI text.** All strings go through the i18n message files.
- **No hard-coded colors.** UI colors come from the palette tokens.
- **No third-party logos** bundled in the repository or the ISO.

## License headers

New source files start with an SPDX header in the file's comment syntax:

```text
SPDX-License-Identifier: GPL-3.0-or-later
```

By contributing you agree that your contribution is released under the same license.
