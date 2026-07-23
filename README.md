# Bast

[![License: MIT](https://img.shields.io/github/license/ellipse-software/bast)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26+-00ADD8?logo=go&logoColor=white)](go.mod)
[![Release](https://img.shields.io/github/v/release/ellipse-software/bast)](https://github.com/ellipse-software/bast/releases)
[![Tests](https://github.com/ellipse-software/bast/actions/workflows/test.yml/badge.svg)](https://github.com/ellipse-software/bast/actions/workflows/test.yml)
[![Website](https://img.shields.io/badge/website-bast.sh-111)](https://bast.sh)

A terminal UI for the OpenSSH you already have. Browse hosts, manage keys, and connect — without a proprietary runtime or a hidden host database.

**Website:** [bast.sh](https://bast.sh) · **Releases:** [github.com/ellipse-software/bast](https://github.com/ellipse-software/bast/releases)

## Install

```sh
curl -fsSL https://bast.sh/install | sh
```

The [bast.sh](https://bast.sh) installer downloads the latest macOS or Linux build for your architecture, verifies the SHA-256 checksum, and installs `bast` to `~/.local/bin` by default. Run the same command again to update; it skips the download when you're already on the latest version.

Set `BAST_INSTALL_DIR` to install somewhere else. Make sure that directory is on your `PATH`, then run:

```sh
bast
```

## Quick start

| Command | What it does |
| --- | --- |
| `bast` | Open the host picker |
| `bast <label>` | Connect directly to a host label |
| `bast "Production web"` | Labels with spaces work — Bast maps them to safe OpenSSH names |

Inside the TUI, press `?` for the full keybinding reference.

## What Bast does

**Hosts** — Reads your existing `~/.ssh/config` (including `Include` files). Add and edit OpenSSH host blocks, favorite and tag them, group them under collapsible headers, hide hosts you rarely use, and search or sort the list.

**Keys** — Generate, import, export, inspect, and delete native SSH keys. Import from a file path or pasted PEM. Verify keypairs, edit public-key comments, and push a public key to a server's `~/.ssh/authorized_keys`.

**Connections** — Launches your system's `ssh` binary with the host's config. Clears the shell before and after sessions, shows a connection banner, and returns you to the picker when the session ends.

Bast adds presentation metadata (groups, tags, colors, notes, favorites, recency) in your OS user config directory. It does not replace OpenSSH as the source of truth for hosts and keys.

## Requirements

- macOS or Linux
- OpenSSH: `ssh`, `ssh-keygen`, `ssh-add`
- For the installer: `curl`, `tar`, and `shasum` or `sha256sum`
- Go 1.26+ when building from source
- A Nerd Font is recommended for some glyphs; Bast works without one

## Keyboard shortcuts

Press `1` for hosts, `2` for keys. Move with arrow keys or `j`/`k`. `/` searches, `r` reloads.

**Hosts:** Enter to connect · `a` add · `e` edit · `d` delete · `f` favorite · `h` hide · `.` toggle hidden · Space collapse/expand group · `s` sort · `K` remove known-host entry

**Keys:** `a` generate · `i` import · `e` edit comment · `d` delete · `u` add to server · `x` export · `p` change passphrase · `c` copy public key

During an SSH session, `exit` returns to Bast. For a stuck session, press Enter then `~.`.

## Files

On first use, Bast prepends one `Include ~/.ssh/bast/config` directive to your main SSH config and writes managed host blocks there. Generated and imported keys live in `~/.ssh/bast/keys` (private keys at mode `0600`). Metadata lives in `bast/state.json` under your user config directory.

Back up `~/.ssh` before testing unreleased builds against production config. Never paste real private keys into issues or bug reports.

## Telemetry

Bast sends optional anonymous telemetry (version, platform, usage events) to help improve the tool. Opt out with:

```sh
export BAST_NO_TELEMETRY=1
```

The [bast.sh](https://bast.sh) site and installer use the same opt-out. Website source: [ellipse-software/bast-web](https://github.com/ellipse-software/bast-web).

## Build from source

```sh
go build -trimpath -o bast .
./bast
```

Release builds can inject a version:

```sh
go build -trimpath -ldflags "-X main.version=v1.0.0" -o bast .
```

## Development

```sh
go mod download
go test -race ./...
go vet ./...
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for project layout and contribution guidelines. Report security issues via [SECURITY.md](SECURITY.md).

## Releases

Tagged releases (`v*`) are built for macOS and Linux on amd64 and arm64, with SHA-256 checksums attached. Push a tag to publish:

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

## License

[MIT](LICENSE)
