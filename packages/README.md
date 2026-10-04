# packages/

**PKGBUILDs** for the `[owneet]` pacman repository: every OwneetOS component (daemon, frontend,
theme, extension, installer, session files) and third-party software not in the official Arch
repositories (for example `xone`, `brave-bin`).

## Build

```text
tools/build-in-vm tools/build-packages            # every package
tools/build-in-vm tools/build-packages NAME...    # only these
```

Packages are built in a **clean chroot** inside the builder VM, against the same Arch Linux
Archive snapshot as the ISO (`iso/pacman.conf`). The result is the `[owneet]` repository in
`out/repo/x86_64/` (packages + `owneet.db`). `tools/build-iso` uses it automatically and builds it
first if it is missing.

## Packages

| Package | Content |
|---------|---------|
| [`owneet-base`](owneet-base/) | OwneetOS release information; depends on `base`. Grows into the meta-package of the system. |

## Rules

- One folder per package, named after the package; sources other than downloads live next to the
  PKGBUILD and have real `sha256sums` (never `SKIP` for local files).
- `license=('GPL-3.0-or-later')` for OwneetOS's own packages.
- The kernel is chosen in `iso/packages.x86_64`, not as a package dependency.

## Signing

Packages and the repository database are signed with the OwneetOS signing subkey
(PROJECT_RULES.md section 5). The key is created once by the owner with
[`tools/keys/create-signing-key`](../tools/keys/create-signing-key); its public part lands in
`owneet-keyring/`. Until then the local repository is used unsigned (`TODO(step 1.2, signing)` in
`tools/build-iso`).
