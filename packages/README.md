# packages/

**PKGBUILDs** for the `[owneet]` pacman repository: every OwneetOS component (daemon, frontend,
theme, extension, installer, session files) and third-party software not in the official Arch
repositories (for example `xone`, `brave-bin`).

- One folder per package, named after the package.
- Packages are built in a clean chroot inside the builder VM and signed.
- The OS updates from this repository (see PROJECT_RULES.md section 5).

Roadmap: step 1.2, then one package per component as it is created.
