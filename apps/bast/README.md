# Bast CLI

The native terminal application for [Bast](https://bast.sh), an SSH host picker and key manager built on OpenSSH.

Bast reads existing SSH configuration, stores its managed hosts in included OpenSSH files, and launches the system `ssh` binary for connections. It also manages SSH keys, imports hosts from shell history, and can sync hosts from GCP, AWS, Azure, and [box.ascii.dev](https://bast.sh/docs/features/box). See the [Bast documentation](https://bast.sh/docs) for installation, usage, and feature guides.

## Development

From the repository root:

```sh
go -C apps/bast mod download
go -C apps/bast build -trimpath -o bast .
go -C apps/bast test -race ./...
go -C apps/bast vet ./...
```

The application is in `apps/bast`. Run `bun run check` from the repository root before submitting changes to verify the CLI and website together.

## Releases

Stable and nightly binaries are built for supported macOS and Linux architectures by the repository's release workflows. Installation options and channel behavior are documented in the [installation guide](https://bast.sh/docs/install).
