# packages/

**PKGBUILDs** for the `[owneet]` pacman repository: every OwneetOS component (daemon, frontend,
theme, extension, installer, session files) and third-party software not in the official Arch
repositories (for example `xone`, `brave-bin`).

## Build

```text
tools/build-in-vm tools/build-packages            # every package
tools/build-in-vm tools/build-packages NAME...    # only these
```

Packages are built in a **clean chroot** inside the builder VM, in dependency order, against the same Arch Linux
Archive snapshot as the ISO (`iso/pacman.conf`). The result is the `[owneet]` repository in
`out/repo/x86_64/` (packages + `owneet.db`). `tools/build-iso` uses it automatically and builds it
first if it is missing.

## Packages

| Package | Content |
|---------|---------|
| [`owneet-base`](owneet-base/) | OwneetOS release information; depends on `base` and `owneet-keyring`. Grows into the meta-package of the system. |
| [`owneet-hardware`](owneet-hardware/) | Firmware, microcode, Mesa + Vulkan (AMD, Intel), NVIDIA driver for Turing and newer with automatic nouveau fallback for older GPUs (`owneet-gpu-select`), PipeWire, NetworkManager, BlueZ, controller hidraw access rules. |
| [`owneet-keyring`](owneet-keyring/) | The OwneetOS public key for pacman (`pacman-key --populate owneet`). |
| [`owneet-branding`](owneet-branding/) | OwneetOS wordmark ("Owneet" + coral "OS", SVG outlines) and the Plymouth boot splash `owneet` (wordmark + coral spinner on deep navy). |
| [`owneet-session`](owneet-session/) | Console user `owneet`, autologin on tty1, `owneet-session` (gamescope or cage, restarts, fallback) and the fullscreen placeholder shown until the real UI exists. |

## Rules

- One folder per package, named after the package; sources other than downloads live next to the
  PKGBUILD and have real `sha256sums` (never `SKIP` for local files).
- `license=('GPL-3.0-or-later')` for OwneetOS's own packages.
- The kernel is chosen in `iso/packages.x86_64`, not as a package dependency.
- After editing a local source file, refresh the checksums: `tools/update-checksums NAME`, and bump
  `pkgver` or `pkgrel`.
- Packages without a `build()` step are built without installing any dependency in the chroot
  (faster, no large downloads, no install scripts of unrelated packages).

## Signing

Packages and the repository database are signed with the OwneetOS signing subkey
(PROJECT_RULES.md section 5). Public key: [`owneet-keyring/owneet.gpg`](owneet-keyring/owneet.gpg),
primary key fingerprint `B3BD F4E3 E477 2D3F 7E86  1A87 8D02 23BC EE51 456E`.

| Who | Command | When |
|-----|---------|------|
| key holder | `tools/keys/install-working-key PATH/owneet-signing-subkey.asc` | once per builder VM |
| key holder | `tools/sign-repo` (asks the passphrase) | after building packages that will be distributed |
| anyone | `tools/build-in-vm tools/build-iso` | verifies signatures if the repository is signed; otherwise warns (development build) |

`tools/build-iso` never reuses OwneetOS packages from pacman's cache, so a stale package or
signature cannot replace a freshly built one.
