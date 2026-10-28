# Release notes

One folder per release, `release-notes/VERSION/`, with one plain-text file per language of the
console (`en.txt`, `it.txt`, `de.txt`, `fr.txt`, `es.txt`): a few short sentences on what the
release changes for the player, in the words of [[writing-for-users]]. The console shows them in
the player's language before updating ([[step-7.1-updates]]); the English ones are also the text
of the GitHub Release.

The assistant drafts them, the owner approves them, and they are part of the release commit
together with the new `pkgver` of `packages/owneet-updates/PKGBUILD`. `tools/release/check`
refuses a release whose notes are missing in any language ([[step-7.2-release-pipeline]]).
