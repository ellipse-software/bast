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

![Bast terminal interface](https://bast.sh/demo.png)

## Install

**Installer script** (macOS and Linux):

```sh
curl -fsSL https://bast.sh/install | sh
```

The [bast.sh](https://bast.sh) installer downloads the latest macOS or Linux build for your architecture, verifies the SHA-256 checksum, and installs `bast` to `~/.local/bin` by default. Run the same command again to update; it skips the download when you're already on the latest version.

Set `BAST_INSTALL_DIR` to install somewhere else.

**Homebrew:**

```sh
brew tap ellipse-software/tap
brew install bast
```

**Build from source** (requires Go 1.26+):

```sh
git clone https://github.com/ellipse-software/bast.git
cd bast
go build -trimpath -o bast .
```

Make sure the install directory is on your `PATH`, then run:

```sh
bast
```

## Quick start

| Command | What it does |
| --- | --- |
| `bast` | Open the host picker |
| `bast update` | Update Bast when it was installed with the bast.sh installer |
| `bast <label>` | Connect directly to a host label |
| `bast "Production web"` | Labels with spaces work. Bast maps them to safe OpenSSH names |
| `bast hosts list` | List hosts without opening the TUI |
| `bast keys list` | List SSH keys without opening the TUI |

Inside the TUI, press `?` for the full keybinding reference.

## Command-line interface

Bare `bast` still opens the TUI. Tagged builds check GitHub for a newer stable release in the background and show a reminder without delaying startup. Installer-managed copies suggest `bast update`, Homebrew copies suggest `brew upgrade bast`, and source builds link back to bast.sh. Network failures are ignored.

`bast update` only runs when the bast.sh installer receipt is present beside the executable. It will not overwrite Homebrew-managed or source-built copies. The `hosts` and `keys` commands expose the same management operations directly to people, scripts, and AI agents.

```sh
# Hosts
bast hosts list --sort group
bast hosts show production
bast hosts add "Production web" --hostname prod.example.com --user deploy --group Work/Production --tag web
bast hosts edit Production_web --notes "Primary application server"
bast hosts favorite Production_web
bast hosts hide old_server
bast hosts known-host remove Production_web
bast hosts delete Production_web

# Keys
bast keys list
bast keys generate work --algorithm ed25519
bast keys import work --private ~/.ssh/id_ed25519
bast keys comment work --comment "Work laptop"
bast keys public work
bast keys install work --host production
bast keys export work --directory ~/Desktop
bast keys passphrase work
bast keys delete work
```

Run `bast hosts <command> --help` or `bast keys <command> --help` for command-specific usage. Host edits are patches: omitted fields stay unchanged, while flags such as `--clear-group`, `--clear-notes`, and `--clear-identity` remove values. Externally managed OpenSSH hosts allow Bast metadata edits but not connection-setting changes or deletion.

Commands prompt for missing input and sensitive confirmations when attached to a terminal. Pass `--yes` to explicitly approve deletion, known-host removal, or private-key export in unattended use. Private keys can be imported from stdin without placing their contents in shell history:

```sh
bast keys import work --private - < id_ed25519
```

Use global `--json` for a stable machine-readable result. It can appear anywhere in the command and disables prompts:

```sh
bast hosts list --json
bast --json hosts show Production_web
bast keys generate automation --no-passphrase --json
```

Successful commands return `{"ok":true,"data":...}`. Errors return `{"ok":false,"error":{"code":"...","message":"..."}}` on stderr and a non-zero exit status. Commands that require an interactive SSH session or passphrase terminal reject `--json` with `interactive_required`.

## What Bast does

**Hosts.** Reads your existing `~/.ssh/config` (including `Include` files). Add and edit OpenSSH host blocks, favorite and tag them, group them under collapsible headers and slash-delimited subgroups up to five levels deep, hide hosts you rarely use, and search or sort the list.

**Keys.** Generate, import, export, inspect, and delete native SSH keys. Import from a file path or pasted PEM. Verify keypairs, edit public-key comments, and push a public key to a server's `~/.ssh/authorized_keys`.

**Connections.** Launches your system's `ssh` binary with the host's config. Clears the shell before and after sessions, shows a connection banner, and returns you to the picker when the session ends.

Bast adds presentation metadata (groups, tags, colors, notes, favorites, recency) in your OS user config directory. It does not replace OpenSSH as the source of truth for hosts and keys.

## Requirements

- macOS or Linux
- OpenSSH: `ssh`, `ssh-keygen`, `ssh-add`
- For the installer: `curl`, `tar`, and `shasum` or `sha256sum`
- Go 1.26+ when building from source
- A Nerd Font is recommended for some glyphs; Bast works without one

## Keyboard shortcuts

Press `1` for hosts, `2` for keys. Move with arrow keys or `j`/`k`. `/` searches, `r` reloads.

Press `v` for the version, credits, website, repository, and license.

**Hosts:** Enter to connect · `a` add · `e` edit · `d` delete · `f` favorite · `h` hide · `.` toggle hidden · Space collapse/expand group · `s` sort · `K` remove known-host entry

**Keys:** `a` generate · `i` import · `e` edit comment · `d` delete · `u` add to server · `x` export · `p` change passphrase · `c` copy public key

During an SSH session, `exit` also closes Bast. For a stuck session, press Enter then `~.` to force-close SSH and return to Bast.

## Files

On first use, Bast prepends one `Include ~/.ssh/bast/config` directive to your main SSH config and writes managed host blocks there. Generated and imported keys live in `~/.ssh/bast/keys` (private keys at mode `0600`). Metadata lives in `bast/state.json` under your user config directory.

Back up `~/.ssh` before testing unreleased builds against production config. Never paste real private keys into issues or bug reports.

## Telemetry

Bast sends optional anonymous telemetry (version, platform, usage events) to help improve the tool. Opt out with:

```sh
export BAST_NO_TELEMETRY=1
```

The [bast.sh](https://bast.sh) site and installer use the same opt-out. Website source: [ellipse-software/bast-web](https://github.com/ellipse-software/bast-web).

## Development

```sh
go mod download
go build -trimpath -o bast .
go test -race ./...
go vet ./...
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for project layout and contribution guidelines. Report security issues via [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
