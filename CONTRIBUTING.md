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

Never include real hostnames, private keys, fingerprints, or production SSH configuration in issues, fixtures, screenshots, or pull requests.

## Release channels

Every CLI change pushed to `master` publishes a rolling nightly pre-release via `.github/workflows/nightly.yml`. Tagged releases (`v*`) are stable and use `.github/workflows/release.yml` instead.

Nightly builds use version strings like `nightly.YYYYMMDD.<sha>`, update the rolling GitHub release tagged `nightly`, and bump the `bast-nightly` Homebrew formula in `ellipse-software/homebrew-tap`.

Stable and nightly installs are mutually exclusive at the same path. Script installs use separate receipts (`https://bast.sh/install` vs `https://bast.sh/install-nightly`); running either installer uninstalls the other channel first (including a Homebrew install of the other formula). The Homebrew formulae also declare `conflicts_with` each other.

Try nightly builds with:

```sh
curl -fsSL https://bast.sh/install-nightly | sh
# or
brew install ellipse-software/tap/bast-nightly
```
