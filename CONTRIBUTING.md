# Contributing to Bast

Thanks for helping improve Bast. Keep changes focused, preserve native OpenSSH behavior, and avoid introducing a second configuration format for hosts or keys.

## Development

You need Go 1.26 and the native `ssh`, `ssh-keygen`, and `ssh-add` commands.

```sh
go mod download
go test -race ./...
go vet ./...
go build -o bast .
```

Run `gofmt` on changed Go files before opening a pull request. Include tests for behavior changes, especially changes that write SSH configuration or private-key files.

## Project layout

- `internal/ui`: TUI lifecycle, loading, input, forms, state, and rendering
- `internal/cli`: non-TUI command dispatch, prompts, text/JSON output, and command tests
- `internal/sshconfig`: native SSH config discovery and Bast-managed host blocks
- `internal/keys`: native key discovery and key operations
- `internal/openssh`: calls to installed OpenSSH tools
- `internal/metadata`: Bast-only presentation metadata

Never include real hostnames, private keys, fingerprints, or production SSH configuration in issues, fixtures, screenshots, or pull requests.

## Release channels

Every push to `master` publishes a rolling nightly pre-release via `.github/workflows/nightly.yml`. Tagged releases (`v*`) are stable and use `.github/workflows/release.yml` instead.

Nightly builds use version strings like `nightly.YYYYMMDD.<sha>`, update the rolling GitHub release tagged `nightly`, and bump the `bast-nightly` Homebrew formula in `ellipse-software/homebrew-tap`.

Stable and nightly installs are mutually exclusive at the same path. Script installs use separate receipts (`https://bast.sh/install` vs `https://bast.sh/install-nightly`); running either installer uninstalls the other channel first (including a Homebrew install of the other formula). The Homebrew formulae also declare `conflicts_with` each other.

Try nightly builds with:

```sh
curl -fsSL https://bast.sh/install-nightly | sh
# or
brew install bast-nightly
```
