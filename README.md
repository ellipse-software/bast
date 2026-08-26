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

Browse SSH hosts, transfer files over SFTP, sync cloud VMs, manage keys, and connect from the terminal.

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

After the repo is configured: `sudo apt install bast`, `sudo dnf install bast`, `sudo pacman -S bast`, or `sudo apk add bast`. Arch users can also `yay -S bast-bin` from the AUR.

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

Requires the Windows OpenSSH Client (`ssh`, `ssh-keygen`, `ssh-add`).

</details>

Then run `bast`. Nightly builds, custom install paths, and building from source are in the [installation guide](https://bast.sh/docs/install).

## Usage

```sh
bast                    # host picker
bast production         # connect by label
bast doctor             # diagnose SSH config and setup
bast hosts list         # list hosts
bast keys list          # list keys
```

Press `?` in the TUI for keybindings. [Documentation](https://bast.sh/docs) covers hosts, keys, Vault, cloud sync, Files, doctor, and the CLI.

## Agent skill

```sh
npx skills add ellipse-software/bast -g -y
```

See [AI agents](https://bast.sh/docs/reference/agents).

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md). Security issues: [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
