# tools/

**Developer scripts**: wrappers that run builds inside the builder VM, release helpers, checks.

Scripts here must never modify the host system; anything that would requires the owner's approval.

Plan: introduced from [[step-0.7-arch-builder-vm]] onwards.

## Frontend preview

`tools/frontend-preview` shows the interface without booting an ISO: after
`tools/build-in-vm tools/build-packages owneet-frontend`, run
`tools/build-in-vm tools/frontend-preview tools/preview-steps/navigation.txt`. It starts the
frontend on a virtual screen in the builder VM with test games and a virtual controller, follows
the steps (keys, controller buttons, sticks, screenshots) and saves the screenshots in
`out/frontend-preview/`. The step format is described at the top of the script.

## Releases

A release is the tag `vX.Y.Z` of a release commit on `main` (the version in
`packages/owneet-updates/PKGBUILD`, its notes in `release-notes/X.Y.Z/`), pushed by an author.
`.github/workflows/release.yml` then waits for an author's approval, builds and signs everything
(`tools/ci/build-iso --release`, which runs `tools/release/build`), waits for a second approval and
publishes (`tools/release/publish`): Arch's source archives and the release's folder on
SourceForge, the packages as the GitHub Release `vX.Y.Z`, and last the signed list of releases.
`tools/release/check X.Y.Z` says whether the current commit is a complete release commit.

To test the whole pipeline without publishing anything, signed with the throwaway test key:
`tools/build-in-vm 'OWNEET_SIGNING_TEST=1 tools/release/build X.Y.Z'` (into `out/release/`).

## Test releases

`tools/build-in-vm tools/make-test-release VERSION` makes a pretend OwneetOS release in
`out/test-release/`, laid out like the GitHub Releases the console reads (the signed
`updates/releases.json` and `vVERSION/` with `owneet-updates` at that version and its repository).
It is signed with a throwaway test key that never leaves the builder VM (only its public half,
`test-key.asc`, comes out), never with the project's keys. To test updates, serve the folder on
`127.0.0.1` and, in the test VM, trust `test-key.asc` (`pacman-key --add`, `--lsign-key`) and set
`URL=http://10.0.2.2:PORT` in `/etc/owneet/updates.conf`.
