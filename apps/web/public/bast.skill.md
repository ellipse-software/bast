---
name: bast
description: >-
  Use Bast CLI for SSH host and key management on macOS, Linux, and Windows 11. Load when the
  user mentions bast, Bast.sh, SSH host picker, ~/.ssh/bast, ssh config
  management, or wants to automate SSH workflows with JSON output.
---

# Bast

Bast is a terminal UI and CLI for browsing SSH hosts, managing keys, and connecting fast. It reads your existing OpenSSH config. It does not replace `ssh` or use a custom protocol.

Docs: https://bast.sh/llms.txt

## When to use Bast

- User wants to browse, search, or organize SSH hosts from the terminal
- User needs to generate, import, export, or install SSH keys
- User wants quick connect: `bast <label>` or `bast "Production web"`
- User wants to import cloud VMs (`bast sync gcp|aws|azure|box|upstash`) and connect with local keys
- User already has the ASCII Box CLI installed and logged in (Bast auto-connects Box)
- User has an Upstash Box API key (Bast stores it locally and uses it for API + SSH)
- User SSHs from a phone or narrow terminal: the TUI switches to a stacked mobile layout below 60 columns (tap Connect)
- Automation/scripts need host or key management with stable JSON output

## When not to use Bast

- PuTTY sessions or `.ppk` keys without OpenSSH conversion
- One-off `ssh user@host` when the host string is already known
- CI provisioning where Terraform/Ansible/IaC owns SSH config
- Non-interactive environments needing TUI (use `bast hosts` / `bast keys` CLI instead)

## Install

Choose one installation method.

Installer:

```sh
curl -fsSL https://bast.sh/install | sh
curl -fsSL https://bast.sh/install | BAST_VERSION=v0.9.1 sh
```

Homebrew:

```sh
brew install ellipse-software/tap/bast
```

Windows 11 PowerShell:

```powershell
irm https://bast.sh/install.ps1 | iex
```

## Automation rules

Always use `--json` for scripts. It disables prompts. Pair with explicit flags and `--yes` for destructive actions.

```sh
bast hosts list --json
bast hosts add "Prod web" --hostname prod.example.com --user deploy --json
bast keys generate automation --no-passphrase --json
bast keys delete old_key --yes --json
```

Success: `{"ok":true,"data":...}` on stdout. Errors: `{"ok":false,"error":{...}}` on stderr with non-zero exit.

Use `--no-input` to never prompt (all required fields must be passed as flags).

## Host commands

```sh
bast hosts list [--search text] [--sort smart|label|recent|group] [--all] [--json]
bast hosts show <host> [--json]
bast hosts add [label] --hostname host [--user u] [--group g] [--tag t] [--json]
bast hosts edit <host> [patch flags] [--json]
bast hosts delete <host> [--yes] [--json]
bast hosts favorite <host> [--json]
bast hosts hide <host> [--json]
bast hosts known-host remove <host> [--yes] [--json]
```

Host edits are **patches**: omitted flags leave values unchanged. Use `--clear-group`, `--clear-notes`, `--clear-identity`, etc. to remove values.

Labels with spaces work: `bast "Production web"`. Aliases are normalized (e.g. `Production_web`).

## Key commands

```sh
bast keys list [--search text] [--json]
bast keys show <name> [--json]
bast keys generate [name] [--algorithm ed25519|rsa] [--no-passphrase] [--json]
bast keys import [name] --private path|- [--public path|-] [--json]
bast keys install <name> --host <host> [--json]
bast keys export <name> --directory path [--yes] [--json]
bast keys delete <name> [--yes] [--json]
```

Import from stdin without shell history: `bast keys import work --private - < id_ed25519`

## Cloud sync

```sh
bast sync gcp
bast sync aws
bast sync azure
bast sync box
bast sync upstash
bast sync status
bast sync disable gcp
bast sync disable box
bast sync disable upstash
```

GCP, AWS, Azure, and ASCII Box require the matching CLI on `PATH` (`gcloud`, `aws`, `az`, or `box`) and an authenticated account. Upstash Box uses a stored API key (`bast upstash key` or `UPSTASH_BOX_API_KEY`), not a CLI. Synced hosts are read-only; disconnect via Sync to remove them.

If the Box CLI is already installed and logged in, Bast auto-connects on TUI start and `bast sync status` (unless you previously ran `bast sync disable box`). The same auto-connect applies to Upstash when a key file is present (unless `bast sync disable upstash`).

On GCP connect, Bast prefers a local key already authorized on the VM. If none matches, it ensures `~/.ssh/google_compute_engine`, publishes it when needed, and may wait for the guest agent.

## Box lifecycle

```sh
bast box new [--type small|default|large] [--ttl seconds | --no-auto-stop] [--no-env]
bast box fork <host|id> [--type small|default|large] [--no-env]
bast box stop <host|id>
bast box resume <host|id> [--type small|default|large] [--no-env]
```

In the TUI, the Box group offers `n` to create and `s` to sync. Stopped boxes are hidden until `.`. Box hosts support Enter to connect (resume first if stopped), `r` resume, `o` stop, and `n` fork. SSH user is always `user` with `~/.ssh/ascii_box_ed25519`. Docs: https://bast.sh/docs/features/box

## Upstash Box lifecycle

```sh
bast upstash key [--key-file path]
bast upstash new [--name name] [--runtime node|python|golang|ruby|rust] [--size small|medium|large] [--keep-alive]
bast upstash fork <host|id>
bast upstash stop <host|id>
bast upstash resume <host|id>
bast upstash delete <host|id> [--yes]
```

SSH user is the box id at `us-east-1.box.upstash.com`. Bast feeds the stored API key as the SSH password. Key file: `~/.config/bast/upstash-box-api-key`. Docs: https://bast.sh/docs/features/upstash

## Files (SFTP)

TUI Files tab (`5`) is a dual-pane local/remote browser over OpenSSH SFTP. Prefer the TUI for interactive transfers; use OpenSSH/`scp`/`sftp` directly when scripting file copies.

TUI tabs: `[1] Hosts` `[2] Keys` `[3] Vault` `[4] Sync` `[5] Files`. Vault is encrypted Bast-managed host/key sync. Sync is cloud VM import (GCP/AWS/Azure/Box/Upstash). Hosted `bast vault login` requires `--accept-terms` under `--json` / `--no-input`.

## File layout

| Path | Purpose |
| --- | --- |
| `~/.ssh/bast/config` | Host blocks created through Bast |
| `~/.ssh/bast/sync/<provider>/config` | Cloud-synced host blocks (while sync is enabled) |
| `~/.ssh/bast/keys/` | Generated/imported keys (private keys mode 0600) |
| `~/.ssh/google_compute_engine` | Fallback GCP identity (created on demand) |
| `~/.config/bast/state.json` | Metadata: groups, tags, colors, notes, favorites, usage stats, sync settings |
| `~/.config/bast/upstash-box-api-key` | Upstash Box API key (mode 0600; not vaulted) |
| `~/.ssh/config` | Gets `Include ~/.ssh/bast/config` on first run |

Connection settings (hostname, user, port, identity files) live in SSH config. Bast metadata (groups, tags, notes) lives in `state.json`.

## Safety

- Never paste private keys into chat, issues, or logs
- Back up `~/.ssh` before testing unreleased builds on a real config
- Bast won't delete externally managed hosts from the main SSH config; it can add metadata edits only
- Commands needing interactive SSH or passphrase entry reject `--json` with `interactive_required`
