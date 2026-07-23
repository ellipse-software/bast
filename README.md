# Bast

Bast is a lightweight terminal interface for native OpenSSH. It gives you a searchable host picker and practical key management without replacing `ssh` or hiding your configuration in a proprietary database.

## Features

- Discover literal host labels from `~/.ssh/config` and its `Include` files
- Connect immediately with Enter or run `bast <label>` directly
- Add and edit ordinary OpenSSH host blocks
- Favorite, group, tag, annotate, sort, and temporarily hide hosts
- Generate, import, export, inspect, and delete native SSH keys
- Import key paths or pasted key contents, derive missing public keys, and verify keypairs
- Edit public-key comments without changing key material or fingerprints
- Return to Bast when the native SSH process exits
- No accounts, network service, telemetry, or custom host/key database

## Requirements

- macOS or Linux
- Go 1.26 or newer when building from source
- OpenSSH commands: `ssh`, `ssh-keygen`, and `ssh-add`
- A Nerd Font is recommended for the return-key glyph, but Bast otherwise remains usable without one

## Build and run

```sh
go build -trimpath -o bast .
./bast
```

Release builds can inject a version with `go build -trimpath -ldflags "-X main.version=v1.0.0" -o bast .`.

Use `bast <label>` to connect without opening the picker. Labels map directly to standard OpenSSH `Host` names.

## Controls

- `1` hosts, `2` keys
- arrows or `j`/`k` move through a list
- left-click a tab or list row to select it
- `/` searches, `r` reloads, and `?` opens full help
- `󰌑` connects to the selected host
- `a`, `e`, and `d` add, edit, and delete where applicable
- `h` hides or shows a host; `.` toggles all hidden hosts
- `q` quits Bast

After Bast launches an SSH session, `exit` returns to the picker. For a stuck OpenSSH session, press Enter and then `~.`.

## Files and safety

- Existing SSH configuration remains authoritative. When Bast first creates a host, it preserves the main config and prepends one top-level `Include ~/.ssh/bast/config` directive.
- Bast-managed host blocks live in `~/.ssh/bast/config`.
- Generated and imported keys live in `~/.ssh/bast/keys` with private files restricted to mode `0600`.
- Favorites, tags, groups, notes, hidden state, and recency live in the operating system user config directory under `bast/state.json`.
- Bast validates key material with the installed `ssh-keygen` and refuses mismatched imported keypairs.

Back up `~/.ssh` before testing unreleased builds against important configuration. Never paste a real private key into a bug report.

## Development

```sh
go mod download
go test -race ./...
go vet ./...
go build -trimpath -o bast .
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the code layout and contribution expectations. Security issues should follow [SECURITY.md](SECURITY.md).

## Releases

The release workflow builds tarballs for macOS and Linux on both Intel and ARM, uploads them as workflow artifacts, and includes SHA-256 checksums.

- Run the workflow manually from GitHub Actions to create downloadable build artifacts without publishing a release.
- Push a `v*` tag to publish the same artifacts as a GitHub Release with generated release notes:

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

## License

[MIT](LICENSE)
