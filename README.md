# Bast

Bast is a lightweight terminal interface for native OpenSSH. It gives you a searchable host picker and practical key management without replacing `ssh` or hiding your configuration in a proprietary database.

## Features

- Discover literal host labels from `~/.ssh/config` and its `Include` files
- Connect immediately with Enter or run `bast <label>` directly
- Add and edit ordinary OpenSSH host blocks
- Use friendly host labels with spaces while Bast keeps underscore-based SSH names behind the scenes
- Use a specific identity, OpenSSH defaults, or password-only authentication per host
- Favorite, tag, annotate, sort, and temporarily hide hosts
- Visually group hosts under collapsible headers, with newly created groups selected after saving
- Generate, import, export, inspect, and delete native SSH keys
- Import key paths or pasted key contents, derive missing public keys, and verify keypairs
- Edit public-key comments without changing key material or fingerprints
- Add a selected public key to a server's `~/.ssh/authorized_keys`
- Keep newly created hosts, groups, and keys in focus after reloading
- Show prominent, descriptive errors while preserving unfinished form input
- Clear the shell before and after SSH sessions, with a connection banner and stuck-session reminder
- Return to Bast when the native SSH process exits
- Automatically dismiss ordinary status notifications after four seconds
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

Use `bast <label>` to connect without opening the picker. Friendly labels containing spaces are converted to safe OpenSSH names with underscores, so `bast "Production web"` connects through `Host Production_web`.

## Controls

- `1` hosts, `2` keys
- arrows or `j`/`k` move through a list
- left-click a tab or list row to select it
- `/` searches, `r` reloads, and `?` opens full help
- `󰌑` connects to the selected host
- `a`, `e`, and `d` add, edit, and delete where applicable
- `␠` (Space) collapses or expands the selected host group
- On a selected key, `u` opens the server picker and installs its public key
- `h` hides or shows a host; `.` toggles all hidden hosts
- `q` quits Bast

### Forms

- Creation forms reveal fields step by step with Enter.
- Edit forms show every field immediately. Use Up/Down to move and Enter to save from anywhere.
- On an editable choice such as Identity, press `␠` to open the choices and Enter to confirm one.
- Destructive confirmations show the exact required text as a muted placeholder; type over it to confirm.
- Errors open in a dedicated panel with the full reason. Enter or Esc returns to the preserved form.

### SSH sessions

- Bast clears the normal shell and scrollback before showing the connection banner and starting OpenSSH.
- `exit` returns to the picker and clears the completed session from the shell again.
- For a stuck session, press Enter and then `~.`. The connection banner repeats this reminder.

### Keys and servers

- Select a key and press `u`, or click **Add to server**, to choose a server and append the public key to its `~/.ssh/authorized_keys`.
- Installation uses the selected OpenSSH host configuration, may prompt for its password, fixes standard SSH file permissions, and does not add duplicate keys.
- Password-only hosts disable public-key authentication. OpenSSH does not store remote-login passwords; install a public key when durable passwordless access is wanted.

## Files and safety

- Existing SSH configuration remains authoritative. When Bast first creates a host, it preserves the main config and prepends one top-level `Include ~/.ssh/bast/config` directive.
- Bast-managed host blocks live in `~/.ssh/bast/config`.
- Generated and imported keys live in `~/.ssh/bast/keys` with private files restricted to mode `0600`.
- Friendly labels, favorites, tags, groups, notes, hidden state, and recency live in the operating system user config directory under `bast/state.json`.
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
