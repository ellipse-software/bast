# Changelog

User-facing changes to Bast (CLI, installers, and behaviour). Website-only changes are not listed.

## Unreleased

### Added

## v0.12.0 - 2026-11-11

### Added

- A host with a local, remote, or dynamic forward can keep those ports up in the background. **Forward** starts `ssh -N` as its own process, so the ports stay up after you quit Bast or open another window. Every window lists the same forwards under a Forwards separator. **Close** or `c` stops them.
- Remote Desktop entries and internal web pages sit in the host list, separate from SSH hosts. `A` adds one. Enter or `Ctrl+O` opens it in the system client. A jump uses an OpenSSH local forward for Remote Desktop and one SOCKS proxy per jump for a page. The Open RDP checkbox on an SSH host is unchanged. `bast rdp` and `bast web` manage them, and Vault syncs them.
- `BAST_NO_SYNC=1` hides Sync and rejects the cloud commands (`bast sync`, `bast boat`, and the other provider commands). `BAST_NO_VAULT=1` hides Vault and rejects `bast vault`. Tabs that remain are numbered from 1 with no gaps. Hosts already on the machine stay usable.
- Files: Left and Right move to the parent directory and into the selected directory. Both panes open on the host picker, with Local at the top and the same groups as Hosts, so either pane can be this machine or any host. A pane shows `local` or the host name only after you choose it. Host, Sync, and Files rows leave one column before the icon. The host form is one scroll: Basics includes authentication, Customisation holds the label, icon, colour, and other metadata, and Advanced still opens on its own.
- Host and group list rows can show an emoji or flag, and a theme ANSI colour or custom hex. `e` on a local group edits the name, icon, and colour. `bast hosts add` and `bast hosts edit` take `--icon` and accept colour names (`red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`, `gray`) or hex.
- About lists the community Discord at discord.gg/bast.
- boxd sync through the `boxd` CLI: import computers from every region of the active org (or an org filter), then create, stop, start, restart, fork, and delete them from the TUI and CLI. Create only targets an org already in that sync. SSH uses the port and key from the entries the boxd CLI keeps in `~/.ssh/config`, so `boxd auth login` must have run. Bast does not store a boxd token. Standby and hibernated computers wake on connect. Fork copies the disk, memory, and running processes.

### Changed

- First launch shows the Bast logo, the slogan, and a short welcome. Any key continues to Hosts, on a history suggestion when one is waiting to import.
- The version screen lists Sponsor, Discord, and Website as buttons. Sponsor starts selected. Left and right move the selection. Enter opens the selected page. `o` on that screen still replays the first-launch chooser.
- The TUI no longer keeps a one-line key footer or a full-screen `?` page. `/` or `?` opens a sheet at the bottom that lists only the actions that apply to the current row, field, or overlay. The status line stays one row. When it is idle, the left side reads “/ for shortcuts”. `Ctrl+F` filters Hosts and Keys, and jumps to a path or searches hosts in Files. On Keys, `n` generates a key and `g` jumps to the top. `Esc` and Backspace no longer quit from the host, key, or vault list. `q` and `Ctrl+C` still quit. A background port forward is `Ctrl+P`.
- Files: Shift+D returns the focused pane to the host picker from a local directory or a connected host. Enter on a group in that picker collapses it when it is open, and expands it when it is closed.
- Colour: Esc leaves a custom hex value and returns to the theme colours. Backspace does the same once the hex field is empty. The picker is a grid of swatches. Arrows move the highlight.

### Fixed

- A sync error on Railway, Fly, DigitalOcean, or boxd shows the warning on that provider's group, the same as the other providers.
- Windows Terminal draws the Enter mark as one cell, so the return icon shows in full. Boat and boxd use one-cell icons there as well.
- Long full-screen errors scroll with the keyboard and the mouse wheel. The actions stay visible at the bottom.

## v0.11.0 - 2026-10-27

### Added

- Open Remote Desktop for an existing host with Ctrl+O. Turn on Open RDP in that host's metadata to show an RDP chip beside Connect. Bast starts the system client and does not take over the terminal.
- BitQate sync over the public API: import VPS instances as SSH hosts, then start, stop, and restart them from the TUI and CLI. The token is read from `BITQATE_TOKEN` or `~/.config/bast/bitqate-token` and is not written to SSH config or Vault. List and sync need `vms:read`. Power actions also need `vms:power`. SSH uses port 22 unless a default port is set. Suspended instances stay distinct from stopped ones and cannot be started from Bast. Instances with no public IPv4 are skipped, and instances removed from the account are removed from the synced SSH config.
- Railway sync over the GraphQL API: import services as SSH hosts, then create, stop, resume, and delete them from the TUI and CLI. SSH is OpenSSH to `ssh.railway.com` as the service instance ID, with a key at `~/.ssh/bast/keys/railway`. Account and workspace tokens work (`RAILWAY_API_TOKEN` or `~/.config/bast/railway-api-token`). Project tokens are rejected. The token is not written to SSH config or Vault. Delete removes the service from that environment only.
- Fly.io sync through `flyctl`: import Machines, then create, stop, start, clone, and destroy them. SSH stays OpenSSH through a local `fly proxy` tunnel (`bast __fly-proxy`). Bast does not store a Fly API token. `flyctl` must be installed and logged in. Cloning a Machine does not copy mounted volumes.
- DigitalOcean sync through `doctl`: import Linux Droplets that have a public address, then create, shut down, power on, fork, and delete them. `bast do` is an alias of `bast digitalocean`. Fork snapshots are deleted after the new Droplet exists. Shutdown keeps the disk billing until delete. Bast does not store a DigitalOcean token. `doctl` auth contexts select the account.

### Changes

- Host forms save with Ctrl+S, or Cmd+S on macOS. Ctrl+Enter still saves.
- In the host list, Right expands the focused group and Left collapses it. Left on a host, or on a group that is already collapsed, moves to the parent group.

### Fixed

- Sync tab Nerd Font icons render in nested WezTerm, Ghostty, and iTerm sessions, and on narrow provider tiles.

Get started at [https://bast.sh](https://bast.sh) or update with `bast update`.

## v0.10.4 - 2026-10-03

- Migrated ASCII Box to Boat across sync, sandbox lifecycle commands, SSH preparation, and diagnostics. Use `bast boat` and `bast sync boat`; previous commands remain aliases. Existing host aliases, metadata, and sync preferences migrate automatically.
- Minor bug fixes and improvements to Bast's TUI and CLI.

Get started at [https://bast.sh](https://bast.sh) or update with `bast update`.

## v0.10.3 - 2026-09-29

- Migrated ASCII Box to Boat across sync, sandbox lifecycle commands, SSH preparation, and diagnostics. Use `bast boat` and `bast sync boat`; previous commands remain aliases. Existing host aliases, metadata, and sync preferences migrate automatically.
- Minor bug fixes and improvements to Bast's TUI and CLI.

Get started at [https://bast.sh](https://bast.sh) or update with `bast update`.

## v0.10.2 - 2026-08-28

- Minor bug fixes and improvements to Bast's TUI and CLI.

Get started at [https://bast.sh](https://bast.sh) or update with `bast update`.

## v0.10.1 - 2026-08-28

- Minor bug fixes and improvements to Bast's TUI and CLI.

Get started at [https://bast.sh](https://bast.sh) or update with `bast update`.

## v0.10.0 - 2026-08-28

### Added

- Vercel Sandbox sync and lifecycle: create, stop, resume, fork, and delete through the REST API. Connect uses a WebSocket PTY (`bast connect`), not OpenSSH. The access token is stored in `~/.config/bast/vercel-token`.
- Host passwords can be stored in Bast and entered on connect. Authentication leads with a key/password picker; User sits on the host hub next to hostname.
- Files tab preview: press `o` or Enter on a file for a floating peek at text, JSON, and PDF contents.
- First TUI launch with no hosts shows a skippable start chooser (add, history, Vault, Sync). Machines that already have SSH hosts skip it. `BAST_NO_ONBOARDING=1` disables it. About (`v`) replays it with `o`.
- About includes a Sponsor chip that opens https://bast.sh/sponsor.
- `bast doctor` diagnoses SSH config, keys, permissions, and Bast setup. Press `D` in the TUI (not in Files). `--fix` repairs the Bast Include and modes OpenSSH will refuse; `--json` exposes stable finding ids.
- `bast doctor` uses the TUI palette on a color terminal (purple headers, red failures, green ok findings). Pipes and `NO_COLOR` stay plain.
- Shell completions via `bast completion` for bash, zsh, fish, PowerShell, Elvish, and Nushell. Tab completes commands, flags, hosts, and keys. The script installers enable this automatically (opt out with `BAST_NO_COMPLETIONS=1`). Homebrew generates bash, zsh, and fish completions on install.
- Script installers accept `BAST_VERSION` (stable) and `BAST_NIGHTLY_VERSION` (nightly) to install a specific GitHub release.
- Keys tab: `g` generates a key, `a` adds the selected key to a server. The in-app `?` overlay, footer hints, and detail chips share one keymap.
- Homebrew formulae compile Bast from source instead of installing prebuilt release archives.
- Hetzner Cloud sync over the Cloud API: import servers, match attached SSH keys, then start, ACPI-stop, and restart from the TUI and CLI. Named API tokens (one per project) are stored in `~/.config/bast/hetzner/tokens/` and are never written to SSH config or Vault. Private Cloud Network addresses are used for SSH when there is no public IP, or when Prefer private IP is on. Non-22 SSH ports are detected when 22 is closed, and an existing local host with the same IP supplies user, port, and identity. The Hetzner Cloud group stays hidden while every server is off.

### Changed

- Empty Hosts and Keys tabs use the same first-run seal as Vault, without narrating keybindings.
- Offline Vercel sandboxes without a snapshot stay off the host list. Cleanup on the Vercel Sync page (or `bast vercel cleanup`) deletes them after confirmation.
- Provider groups with no running hosts stay hidden until `.` reveals stopped or hidden hosts.

### Fixed

- `bast doctor` finds the ASCII Box CLI at `~/.ascii/bin/box` the same way Bast sync does, instead of reporting it missing when `box` is only a shell function.

## v0.9.3 - 2026-08-23

### Added

- Linux packages for apt, dnf, pacman, and apk: `.deb`, `.rpm`, `.apk`, and Arch packages on GitHub releases, plus a signed repository at https://packages.bast.sh.

### Fixed

- Box stop returns once the box is stopping instead of waiting minutes for the snapshot to finish, and a late sync result can no longer clear a newer in-flight Box operation.
- Hosts provider group selection fills the highlight under branded labels and icons.
- Stop, fork, resume, and delete work on a selected sandbox on the Sync provider page, matching Hosts.

## v0.9.2 - 2026-08-23

### Added

- Upstash Box sync and lifecycle: create, pause, resume, fork, and delete through the Box API. The API key is stored in `~/.config/bast/upstash-box-api-key` and used as the SSH password.

### Changed

- Unlinked Vault tab shows a first-run seal instead of a two-line stub.
- Hosts and Vault primary actions sit in a chip row under the title, matching Sync.
- Hosts detail action chips have a blank line above and below.

Get started at [https://bast.sh](https://bast.sh) or update with `bast update`.

## v0.9.1 - 2026-08-23

### Added

- Sync tab (`4`) for AWS, GCP, Azure, and box.ascii.dev, separate from Vault.
- Grid layout in the Sync tab.
- Improved Vault tab UX.
- Terms of service dialog for the hosted service.

### Fixed

- Windows PowerShell installer first-run output and telemetry notice.
- `bast update` on Windows writes the installer script with a UTF-8 BOM so PowerShell can run it.
- Inactive VMs from ASCII Box are hidden automatically.
- Switching between hidden and visible hosts no longer jumps the cursor to the top.

Get started at [https://bast.sh](https://bast.sh) or update with `bast update`.

## v0.9.0 - 2026-08-19

Bast v0.9.0 adds native Windows 11 support alongside macOS and Linux.

## Highlights

- Native Windows AMD64 and ARM64 builds
- Authenticode-signed Windows executables
- PowerShell installer with checksum and signature verification
- Windows-aware SSH configuration, keys, history, Vault, SFTP, and updates
- WinGet manifests included for submission

## Install

### Windows 11

```powershell
irm https://bast.sh/install.ps1 | iex
```

Windows OpenSSH Client is required.

### macOS and Linux

```bash
curl -fsSL https://bast.sh/install | sh
```

Or with Homebrew:

```bash
brew install ellipse-software/tap/bast
```

## Windows notes

Native Windows and WSL maintain separate SSH configuration and Bast state.
Vault can be used to synchronise Bast-managed hosts and keys between them.

WinGet installation will become available after the manifests are accepted into
microsoft/winget-pkgs.

## Upgrade

Existing script and Homebrew installations can be upgraded normally:

```bash
bast update
```

## Full changelog

https://github.com/ellipse-software/bast/compare/v0.8.1...v0.9.0

## v0.8.1 - 2026-08-11

**Full Changelog**: https://github.com/ellipse-software/bast/compare/v0.8.0...v0.8.1

## v0.8.0 - 2026-08-06

## What's Changed
* feat: box.ascii.dev by @tedbrine in https://github.com/ellipse-software/bast/pull/51

**Full Changelog**: https://github.com/ellipse-software/bast/compare/v0.7.1...v0.8.0

## v0.7.1 - 2026-08-01

**Full Changelog**: https://github.com/ellipse-software/bast/compare/v0.7.0...v0.7.1

## v0.7.0 - 2026-08-01

## What's Changed
* feat: sftp by @tedbrine in https://github.com/ellipse-software/bast/pull/42
* feat: vault by @tedbrine in https://github.com/ellipse-software/bast/pull/43
* feat: chmod and info by @tedbrine in https://github.com/ellipse-software/bast/pull/46

**Full Changelog**: https://github.com/ellipse-software/bast/compare/v0.6.6...v0.7.0
