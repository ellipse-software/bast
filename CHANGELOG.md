# Changelog

User-facing changes to Bast (CLI, installers, and behaviour). Website-only changes are not listed.

## Unreleased

### Added

- Railway sync and lifecycle: import services as SSH hosts, plus create, stop, resume, and delete through the GraphQL API. The API token is stored in `~/.config/bast/railway-api-token`. SSH uses `~/.ssh/bast/keys/railway` against `ssh.railway.com`.

### Changed

- Empty cloud provider groups no longer appear on the Hosts tab. If every host is stopped or sleeping, the group stays hidden until `.`.

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
