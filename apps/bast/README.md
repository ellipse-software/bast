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

[Home](https://bast.sh) · [Releases](https://github.com/ellipse-software/bast/releases) · [Contributing](../../CONTRIBUTING.md) · [Security](../../SECURITY.md)

[![License: MIT](https://img.shields.io/github/license/ellipse-software/bast)](../../LICENSE)
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
brew install ellipse-software/tap/bast
```

### Nightly

```sh
curl -fsSL https://bast.sh/install-nightly | sh
```

```sh
brew install ellipse-software/tap/bast-nightly
```

Script installs switch channels automatically: installing nightly removes a stable install at the same path (and vice versa), including Homebrew installs of the other formula.

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

Press `?` in the TUI for keybindings. Installed via Homebrew? Use `brew upgrade bast` instead of `bast update`.

## CLI

If you aren't a fan of the TUI, you can also use bast directly in your terminal.

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

Run `bast hosts <command> --help`, `bast keys <command> --help`, or `bast sync <command> --help` for the full flag list.

### Cloud sync

The Sync tab (`3` in the TUI) can import SSH-ready virtual machines from Google Cloud, AWS, and Azure. Synced hosts are read-only in Bast and remain ordinary OpenSSH entries under `~/.ssh/bast/sync`.

```sh
bast sync gcp
bast sync aws
bast sync azure
bast sync status
bast sync disable gcp
bast sync disable aws
bast sync disable azure
```

GCP sync requires the Google Cloud SDK (`gcloud`) and an authenticated account (`gcloud auth login`). Bast scans accessible projects, imports running Compute Engine instances, and groups them under `Google Cloud/<project>`. Project filters and optional service-account JSON keys are available in the GCP Sync screen. Public instances connect directly. Private instances use IAP tunnelling via `gcloud compute start-iap-tunnel` when needed. Bast prefers a matching local identity (`~/.ssh/google_compute_engine` or metadata keys); otherwise it can publish a short-lived key through instance or project metadata. Bast does not create projects, change firewall rules, or start instances. Narrowing the project filter removes previously synced hosts from newly excluded projects on the next sync.

AWS sync requires AWS CLI v2 and at least one configured profile. By default Bast scans every configured profile and enabled region; profile and region filters are available in the AWS Sync screen. Discovery uses `sts:GetCallerIdentity`, `ec2:DescribeRegions`, `ec2:DescribeInstances`, `ec2:DescribeImages`, and `ec2:DescribeInstanceConnectEndpoints`. Bast connects directly to public addresses. Private instances require an active EC2 Instance Connect Endpoint and `ec2-instance-connect:OpenTunnel`; these tunnels have AWS's one-hour maximum duration.

When the instance's launch key exists in `~/.ssh/bast/keys` or `~/.ssh`, Bast uses it. Otherwise it creates `~/.ssh/bast/aws_compute` and publishes the public key immediately before connecting, which requires `ec2-instance-connect:SendSSHPublicKey` and EC2 Instance Connect support on the instance. Bast does not create endpoints, change security groups, or start instances.

Azure sync requires Azure CLI 2.62 or later and an authenticated `az login`. Bast scans enabled subscriptions, imports running standalone Linux VMs, and groups them under `Microsoft Azure/<subscription>/<resource group>`. Subscription and resource-group filters are available in the Azure Sync screen.

Public Azure VMs connect directly. Private VMs require an existing Standard or Premium Azure Bastion with native-client tunnelling enabled and the `bastion` Azure CLI extension. Bast prefers a matching local deployment key; otherwise an Entra-enabled VM can use a short-lived OpenSSH certificate through the `ssh` Azure CLI extension. Bast does not create Bastion resources, install VM extensions, change networks, or append keys to VMs.

Edits are patches. Only the flags you pass change. `--clear-group`, `--clear-notes`, and similar flags remove values. Hosts that only exist in your main SSH config can get Bast metadata edits, but Bast won't touch their connection settings or delete them.

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

On startup, Bast checks standard zsh (`~/.zsh_history`, `~/.zhistory`, `$HISTFILE`, and `$ZDOTDIR`), bash, and fish history files in the background for SSH destinations that are not already configured. Suggestions appear below your hosts under `(Suggested)`. Press Enter to add one, `e` to review it first, or `x` to dismiss it. Bast stores normalized connection fields and scan checkpoints, not the original history command.

Groups can nest up to five levels. Set them in the label with a path (`Work/Production/web` puts the host in `Work/Production` and names it `web`). In the TUI, the inactive Label row shows just the name; focusing it reveals the full path, with the group prefix muted until the cursor moves into it. `--group` remains available on the CLI. Bast stores presentation metadata in `~/.config/bast`. OpenSSH stays the source of truth for hosts and keys.

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

`1` hosts · `2` keys · `3` sync · arrows or `j`/`k` to move · `g`/`G` top/bottom · `/` search · `r` reload · `v` version info

**Hosts:** Enter connect/add suggestion · `a` add · `e` edit/review suggestion · `x` dismiss suggestion · `d` delete · `f` favorite · `h` hide · `.` show hidden · Space collapse/expand group · `[` collapse all · `]` expand all · `s` sort · `K` drop known-host entry

**Keys:** `a` generate · `i` import · `e` edit comment · `d` delete · `u` push to server · `x` export · `p` passphrase · `c` copy public key

**Sync:** Enter open provider / run action · Esc back · `r` refresh

During a session, `exit` returns to Bast. Stuck? Enter, then `~.` to force SSH closed.

## Files

On first run Bast adds `Include ~/.ssh/bast/config` to your SSH config and writes managed host blocks there. Keys live in `~/.ssh/bast/keys`. Metadata is in `~/.config/bast/state.json`.

Back up `~/.ssh` before trying unreleased builds on a real config. Don't paste private keys in issues.

## Telemetry

Anonymous usage telemetry (version, platform, events) is on by default. Opt out:

```sh
export BAST_NO_TELEMETRY=1
```

When an error occurs in an interactive session, Bast can offer to send an error report that may include error text and stack traces. In the TUI error overlay, Space sends the report; Enter, Esc, Backspace, and Ctrl+H dismiss it; q quits; unrelated keys leave it open. After a failing CLI command in a terminal, Bast prints a one-key prompt: Space sends the report, and any other key continues immediately. SSH session endings are not reported. `BAST_NO_TELEMETRY=1` disables both usage telemetry and error reports.

## Development

From the repository root:

```sh
go -C apps/bast mod download
go -C apps/bast build -trimpath -o bast .
go -C apps/bast test -race ./...
go -C apps/bast vet ./...
```

See [CONTRIBUTING.md](../../CONTRIBUTING.md). Security issues: [SECURITY.md](../../SECURITY.md).

## License

[MIT](../../LICENSE)
