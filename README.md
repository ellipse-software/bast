<div align="center">

<h1>
  <a href="https://bast.sh">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://cdn.bast.sh/bast-long-word.png">
      <img alt="Bast" src="https://cdn.bast.sh/bast-long-word.png" width="600">
    </picture>
  </a>
</h1>

**The fast way into the servers you use every day.**

Browse SSH hosts, open Remote Desktop targets and internal pages, transfer files over SFTP, sync cloud VMs, manage keys, and connect from the terminal.

[Home](https://bast.sh) · [Docs](https://bast.sh/docs) · [Discord](https://discord.gg/bast) · [Releases](https://github.com/ellipse-software/bast/releases) · [Contributing](./CONTRIBUTING.md) · [Security](./SECURITY.md)

[![License: MIT](https://img.shields.io/github/license/ellipse-software/bast)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26+-00ADD8?logo=go&logoColor=white)](apps/bast/go.mod)
[![Release](https://img.shields.io/github/v/release/ellipse-software/bast)](https://github.com/ellipse-software/bast/releases)
[![Tests](https://github.com/ellipse-software/bast/actions/workflows/test.yml/badge.svg)](https://github.com/ellipse-software/bast/actions/workflows/test.yml)
[![Stars](https://img.shields.io/github/stars/ellipse-software/bast?style=flat&logo=github&label=Stars&color=181717)](https://github.com/ellipse-software/bast/stargazers)
[![Sponsor](https://img.shields.io/badge/Sponsor-EA4AAA?logo=githubsponsors&logoColor=white)](https://github.com/sponsors/tedbrine)

</div>

![Bast terminal interface](https://bast.sh/demo.png?12)

## Install

<details>
<summary>macOS</summary>

```sh
curl -fsSL https://bast.sh/install | sh
```

Or with Homebrew:

```sh
brew install ellipse-software/tap/bast
```

</details>

<details>
<summary>Linux</summary>

```sh
curl -fsSL https://bast.sh/install | sh
```

Package managers (adds the Bast repo, then installs):

```sh
curl -fsSL https://packages.bast.sh/setup.sh | sudo sh
```

After the repo is configured: `sudo apt install bast`, `sudo dnf install bast` (or `sudo yum install bast`), `sudo zypper install bast`, `sudo pacman -S bast`, or `sudo apk add bast`. Arch users can also `yay -S bast-bin` from the AUR.

Or with Homebrew:

```sh
brew install ellipse-software/tap/bast
```

</details>

<details>
<summary>Windows 11</summary>

```powershell
irm https://bast.sh/install.ps1 | iex
```

Or with WinGet:

```powershell
winget install EllipseSoftware.Bast
```

Update WinGet installs with `winget upgrade --id EllipseSoftware.Bast --exact`. New releases become available after review in the WinGet community repository.

Requires the Windows OpenSSH Client (`ssh`, `ssh-keygen`, `ssh-add`).

</details>

Then run `bast`. Nightly builds, custom install paths, and building from source are in the [installation guide](https://bast.sh/docs/install).

## Update

`bast update` updates script installs from [bast.sh](https://bast.sh), including nightly and the Windows PowerShell installer. It does not replace a copy installed by a package manager.

| Install | Command |
| --- | --- |
| bast.sh script, nightly script, or Windows PowerShell installer | `bast update` |
| Homebrew | `brew upgrade bast` |
| Homebrew nightly | `brew upgrade bast-nightly` |
| apt | `sudo apt upgrade bast` |
| dnf | `sudo dnf upgrade bast` |
| yum | `sudo yum update bast` |
| zypper | `sudo zypper update bast` |
| pacman | `sudo pacman -Syu bast` |
| AUR (`bast-bin`) | `yay -Syu bast-bin` when yay is on `PATH`, otherwise `paru -Syu bast-bin`, otherwise `sudo pacman -Syu bast-bin` |
| apk | `sudo apk upgrade bast` |
| WinGet | `winget upgrade --id EllipseSoftware.Bast --exact` |

Stable builds check for a newer stable release. Nightly builds check the nightly channel. When one exists, the idle status line shows the version and the command for your install. Press `v` for the same suggestion. A source build links to [bast.sh](https://bast.sh). WinGet can lag GitHub while the community repository reviews the release. Details: [Updates](https://bast.sh/docs/reference/updates).

## Usage

```sh
bast                    # host picker
bast production         # connect by label
bast doctor             # diagnose SSH config and setup
bast hosts list         # list hosts
bast keys list          # list keys
bast rdp list           # Remote Desktop entries
bast web list           # internal web pages
bast update             # script installs only
```

Press `/` in the TUI for the actions available right now. [Documentation](https://bast.sh/docs) covers hosts, keys, Remote Desktop, web pages, Vault, cloud sync, Files, doctor, and the CLI.

## Agent skill

```sh
npx skills add ellipse-software/bast -g -y
```

See [AI agents](https://bast.sh/docs/reference/agents).

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues: [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
