# Contributing to Bast

Thanks for helping improve Bast. Keep changes focused, preserve native OpenSSH behavior, and avoid introducing a second configuration format for hosts or keys.

## Development

You need Go 1.26, Bun 1.3+, and the native `ssh`, `ssh-keygen`, and `ssh-add` commands.

```sh
bun install
bun run check
```

Run the web development server with `bun run dev`. Build the CLI alone with `bun run build:bast`.

Run `gofmt` on changed Go files before opening a pull request. This repo ships a pre-commit hook that auto-formats staged `.go` files:

```sh
git config core.hooksPath .githooks
```

CI still enforces `test -z "$(gofmt -l .)"` within `apps/bast`. Include tests for behavior changes, especially changes that write SSH configuration or private-key files.

## Project layout

- `apps/bast`: Go CLI and terminal interface
- `apps/bast/internal`: CLI, TUI, OpenSSH, metadata, cloud, telemetry, and updater packages
- `apps/web`: Next.js site, documentation, installer scripts, telemetry, and error-reporting routes
- `skills/bast`: Canonical agent skill (`SKILL.md`) for `npx skills add ellipse-software/bast`

After editing `skills/bast/SKILL.md`, run `bun run sync:skill` so `apps/web/public/bast.skill.md` and its checksum stay in sync (also run by `bun run check`).

Never include real hostnames, private keys, fingerprints, or production SSH configuration in issues, fixtures, screenshots, or pull requests.

## Release channels

Every CLI change pushed to `master` publishes a rolling nightly pre-release via `.github/workflows/nightly.yml`. Tagged releases (`v*`) are stable and use `.github/workflows/release.yml` instead.

Nightly builds use version strings like `nightly.YYYYMMDD.<sha>`, update the rolling GitHub release tagged `nightly`, and bump the `bast-nightly` Homebrew formula in `ellipse-software/homebrew-tap`.

Stable tags also build `.deb`, `.rpm`, `.apk`, and Arch packages and publish them to the signed repository at `https://packages.bast.sh`. Nightly is script and Homebrew only. Publishing the repo requires `LINUX_SIGNING_PRIVATE_KEY`, `LINUX_SIGNING_PUBLIC_KEY`, `PACKAGES_R2_ACCOUNT_ID`, `PACKAGES_R2_ACCESS_KEY_ID`, and `PACKAGES_R2_SECRET_ACCESS_KEY` (optional `LINUX_SIGNING_PASSPHRASE`, `LINUX_APK_RSA_*`, `PACKAGES_R2_BUCKET`).

Stable and nightly installs are mutually exclusive at the same path. Script installs use separate receipts (`https://bast.sh/install` vs `https://bast.sh/install-nightly`); running either installer uninstalls the other channel first (including a Homebrew install of the other formula). The Homebrew formulae also declare `conflicts_with` each other.

Try nightly builds with:

```sh
curl -fsSL https://bast.sh/install-nightly | sh
# or
brew install ellipse-software/tap/bast-nightly
```

## Changelog

User-facing product changes (CLI, installers, updater, and behavior) add a bullet under `Unreleased` in `CHANGELOG.md` in the same change. Website-only work does not.

`bash .github/scripts/changelog.sh preview` prints commits since the last stable tag next to the current Unreleased notes.

## Stable release

1. Review notes: `bash .github/scripts/changelog.sh preview`
2. Cut the version: `bash .github/scripts/changelog.sh cut vX.Y.Z`
3. Commit `chore: release vX.Y.Z` and push to `master`
4. Tag and push `vX.Y.Z`

The tag workflow publishes the GitHub release body from that changelog section. Publishing fails if the section is missing or empty. Nightly notes are unchanged.
