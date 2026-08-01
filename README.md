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

[Home](https://bast.sh) · [Docs](https://bast.sh/docs) · [Releases](https://github.com/ellipse-software/bast/releases) · [Contributing](./CONTRIBUTING.md) · [Security](./SECURITY.md)

[![License: MIT](https://img.shields.io/github/license/ellipse-software/bast)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26+-00ADD8?logo=go&logoColor=white)](apps/bast/go.mod)
[![Release](https://img.shields.io/github/v/release/ellipse-software/bast)](https://github.com/ellipse-software/bast/releases)
[![Tests](https://github.com/ellipse-software/bast/actions/workflows/test.yml/badge.svg)](https://github.com/ellipse-software/bast/actions/workflows/test.yml)
[![Stars](https://img.shields.io/github/stars/ellipse-software/bast?style=flat&logo=github&label=Stars&color=181717)](https://github.com/ellipse-software/bast/stargazers)
[![Sponsor](https://img.shields.io/badge/Sponsor-EA4AAA?logo=githubsponsors&logoColor=white)](https://github.com/sponsors/tedbrine)

</div>

![Bast terminal interface](https://bast.sh/demo.png?a)

## Install

**macOS and Linux:**

```sh
curl -fsSL https://bast.sh/install | sh
```

This installs `bast` to `~/.local/bin` by default. See [Installation](https://bast.sh/docs/reference/install) for custom locations, updates, and other options.

**Homebrew:**

```sh
brew install ellipse-software/tap/bast
```

### Nightly

```sh
curl -fsSL https://bast.sh/install-nightly | sh
```

```sh
brew install ellipse-software/tap/bast-nightly
```

See [Installation](https://bast.sh/docs/reference/install#nightly-channel) for nightly channel behavior.

### From Source

```sh
git clone https://github.com/ellipse-software/bast.git
cd bast/apps/bast
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

Press `?` in the TUI for keybindings. See the [documentation](https://bast.sh/docs) for setup and feature guides.

## CLI

Bast also provides commands for managing hosts and keys without opening the TUI.

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

Run `bast hosts <command> --help`, `bast keys <command> --help`, or `bast sync <command> --help` for available flags. The [command-line guide](https://bast.sh/docs/features/cli) covers automation, JSON output, prompts, and edit behavior.

## Cloud sync

The Sync tab (`3` in the TUI) and `bast sync` commands import read-only virtual machines from [GCP](https://bast.sh/docs/features/gcp), [AWS](https://bast.sh/docs/features/aws), and [Azure](https://bast.sh/docs/features/azure). Each provider guide covers prerequisites, filters, private networking, authentication, and the changes Bast will and will not make.

## Vault

[Vault](https://bast.sh/docs/features/vault) syncs Bast-managed hosts and keys between machines with end-to-end encryption (`bast vault login`). Cloud VM inventory still re-syncs per machine via provider CLIs.

## What Bast does

Bast reads your existing OpenSSH configuration, adds organization and key management, discovers hosts from shell history, transfers files over SFTP, and launches the system `ssh` binary for connections. OpenSSH remains the source of truth. Start with the [documentation](https://bast.sh/docs), or read about [host management](https://bast.sh/docs/features/host-management), [Files](https://bast.sh/docs/features/files), [history import](https://bast.sh/docs/features/history-import), and [SSH keys](https://bast.sh/docs/features/keys).

## Agent skill

Teach Cursor, Claude Code, Codex, and other agents how to use Bast:

```sh
npx skills add ellipse-software/bast -g -y
```

See [AI agents](https://bast.sh/docs/reference/agents) for project-local install and curl fallbacks. The skill source is [`skills/bast/SKILL.md`](./skills/bast/SKILL.md).

## Requirements

- macOS or Linux
- OpenSSH (`ssh`, `ssh-keygen`, `ssh-add`)
- Google Cloud SDK (`gcloud`) for GCP cloud sync
- AWS CLI v2 for AWS cloud sync
- Azure CLI 2.62+ for Azure cloud sync
- `curl`, `tar`, and `shasum` or `sha256sum` for the installer
- Go 1.26+ to build from source
- A Nerd Font is detected automatically in WezTerm, Kitty, iTerm2, Alacritty, Ghostty, Warp, and Windows Terminal for cloud icons; use `BAST_NERD_FONT=1` or `BAST_NERD_FONT=0` to override detection

## Keyboard shortcuts

Press `?` in Bast to see contextual keybindings, or use the [keyboard shortcut reference](https://bast.sh/docs/features/shortcuts).

## Files

The Files tab (`4` in the TUI) is a dual-pane local/remote browser over OpenSSH SFTP. See [Files](https://bast.sh/docs/features/files).

For paths Bast writes under `~/.ssh`, see [Files and storage](https://bast.sh/docs/reference/files). Back up `~/.ssh` before trying unreleased builds on a real config, and never paste private keys into issues.

## Telemetry

Anonymous usage telemetry is on by default. Opt out with:

```sh
export BAST_NO_TELEMETRY=1
```

This also disables optional error reports. See [Telemetry and error reports](https://bast.sh/docs/reference/telemetry) for what is collected and when consent is requested.

## Development

From the repository root:

```sh
bun install
bun run check
bun run build
```

Run `bun run dev` for the website. The Go CLI is in [`apps/bast`](apps/bast), and the Next.js site is in [`apps/web`](apps/web).

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues: [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
