<div align="center">

<h1>
  <a href="https://bast.sh">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://cdn.bast.sh/bast-word-dark.png?d">
      <img alt="Bast" src="https://cdn.bast.sh/bast-word.png?d" width="250">
    </picture>
  </a>
</h1>

**The fast way into the servers you use every day.**

Browse SSH hosts, manage keys, and connect from the terminal.

[Home](https://bast.sh) · [Releases](https://github.com/ellipse-software/bast/releases) · [Contributing](./CONTRIBUTING.md) · [Security](./SECURITY.md)

[![License: MIT](https://img.shields.io/github/license/ellipse-software/bast)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26+-00ADD8?logo=go&logoColor=white)](go.mod)
[![Release](https://img.shields.io/github/v/release/ellipse-software/bast)](https://github.com/ellipse-software/bast/releases)
[![Tests](https://github.com/ellipse-software/bast/actions/workflows/test.yml/badge.svg)](https://github.com/ellipse-software/bast/actions/workflows/test.yml)

</div>

![Bast terminal interface](https://bast.sh/demo.png?a)

## Install

**macOS and Linux:**

```sh
curl -fsSL https://bast.sh/install | sh
```

Puts `bast` in `~/.local/bin` by default. Run the same command to update. Set `BAST_INSTALL_DIR` to install somewhere else.

**Homebrew:**

```sh
brew tap ellipse-software/tap
brew install bast
```

**Nightly** (latest `master`; cannot coexist with stable):

```sh
curl -fsSL https://bast.sh/install-nightly | sh
```

```sh
brew install bast-nightly
```

**From source** (Go 1.26+):

```sh
git clone https://github.com/ellipse-software/bast.git
cd bast
go build -trimpath -o bast .
```

Put the binary on your `PATH`, then run `bast`.

## Quick start

```sh
bast                    # open the host picker
bast update             # update (script installs only)
bast production         # connect by label
bast "Production web"   # labels with spaces work too
bast hosts list         # list hosts without the TUI
bast keys list          # list keys without the TUI
```

Press `?` in the TUI for keybindings. Installed via Homebrew? Use `brew upgrade bast` instead of `bast update`.

## CLI

No args opens the TUI. Host and key management lives under `bast hosts` and `bast keys`:

```sh
bast hosts list --sort group
bast hosts add "Production web" --hostname prod.example.com --user deploy
bast hosts edit Production_web --notes "Primary app server"
bast hosts delete Production_web

bast keys generate work --algorithm ed25519
bast keys import work --private ~/.ssh/id_ed25519
bast keys install work --host production
bast keys delete work
```

Run `bast hosts <command> --help` or `bast keys <command> --help` for the full flag list.

Edits are patches — only the flags you pass change. `--clear-group`, `--clear-notes`, and similar flags remove values. Hosts that only exist in your main SSH config can get Bast metadata edits, but Bast won't touch their connection settings or delete them.

Commands prompt when they need input. `--yes` skips confirmations for delete, known-host removal, and key export. Import from stdin without putting the key in shell history:

```sh
bast keys import work --private - < id_ed25519
```

`--json` anywhere in the command gives script-friendly output and turns off prompts:

```sh
bast hosts list --json
bast keys generate automation --no-passphrase --json
```

Success: `{"ok":true,"data":...}`. Errors go to stderr as `{"ok":false,"error":{...}}` with a non-zero exit code.

## What Bast does

Bast is a terminal UI for the SSH config and keys you already have. It reads `~/.ssh/config` (including `Include` files), lets you organize hosts with groups, tags, favorites, and notes, and connects by launching your system `ssh`.

You can generate and manage keys, push public keys to servers, and do most of it from the CLI if you'd rather skip the TUI.

Groups can nest up to five levels (`Work/Production/web`). Bast stores presentation metadata in your user config directory. OpenSSH stays the source of truth for hosts and keys.

## Requirements

- macOS or Linux
- OpenSSH (`ssh`, `ssh-keygen`, `ssh-add`)
- `curl`, `tar`, and `shasum` or `sha256sum` for the installer
- Go 1.26+ to build from source
- A Nerd Font helps with some icons but isn't required

## Keyboard shortcuts

`1` hosts · `2` keys · arrows or `j`/`k` to move · `/` search · `r` reload · `v` version info

**Hosts:** Enter connect · `a` add · `e` edit · `d` delete · `f` favorite · `h` hide · `.` show hidden · Space collapse group · `s` sort · `K` drop known-host entry

**Keys:** `a` generate · `i` import · `e` edit comment · `d` delete · `u` push to server · `x` export · `p` passphrase · `c` copy public key

During a session, `exit` closes Bast too. Stuck? Enter, then `~.` to force SSH closed.

## Files

On first run Bast adds `Include ~/.ssh/bast/config` to your SSH config and writes managed host blocks there. Keys live in `~/.ssh/bast/keys`. Metadata is in `bast/state.json` under your user config dir.

Back up `~/.ssh` before trying unreleased builds on a real config. Don't paste private keys in issues.

## Telemetry

Anonymous usage telemetry (version, platform, events) is on by default. Opt out:

```sh
export BAST_NO_TELEMETRY=1
```

Same opt-out on [bast.sh](https://bast.sh). Website repo: [ellipse-software/bast-web](https://github.com/ellipse-software/bast-web).

## Development

```sh
go mod download
go build -trimpath -o bast .
go test -race ./...
go vet ./...
```

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues: [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
