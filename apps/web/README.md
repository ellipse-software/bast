# Bast.sh

The landing page and installer for [Bast](https://github.com/ellipse-software/bast), a native SSH host picker and key manager.

The installer is served from `public/install` (stable) and `public/install-nightly` (nightly). Each downloads the matching macOS or Linux build for the current architecture, verifies its SHA-256 checksum, and installs `bast` to `~/.local/bin` by default. It leaves an installation receipt beside the binary so installer-managed copies can run `bast update`; Homebrew and source builds are not eligible for self-update. Run the same install command again to update Bast or create the receipt for an existing install; it exits without downloading when the installed version is already current. Installing one channel uninstalls the other first (script install at the same path, or the other Homebrew formula). Set `BAST_INSTALL_DIR` to choose another location. After install, the script writes shell completion files and a marked block in the user's shell startup file. Set `BAST_NO_COMPLETIONS=1` to skip that.

Telemetry events from the installer and Bast CLI are sent to `/api/telemetry` and forwarded to PostHog. Consented error reports from the CLI are sent to `/api/errors` and forwarded via the Sentry SDK to Better Stack. Set `POSTHOG_API_KEY` (and optionally `POSTHOG_HOST`) and `SENTRY_DSN` in the deployment environment. Users can opt out with `BAST_NO_TELEMETRY=1`.

Vault (encrypted Bast config sync) needs Upstash Redis (`UPSTASH_REDIS_REST_URL`, `UPSTASH_REDIS_REST_TOKEN`), Cloudflare R2 (`R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET_NAME`), and Cloudflare Email Sending via `@opencoredev/email-sdk` (`CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID` or reuse `R2_ACCOUNT_ID`, optional `CLOUDFLARE_EMAIL_FROM`). Enable Email Sending for your From domain in Cloudflare before production sends. See `.env.example`. For running your own instance, see the [self-hosting docs](https://bast.sh/docs/reference/self-hosting).

Health checks:

- `GET /api/health` — marketing app liveness
- `GET /api/health/docs` — docs content source loads
- `GET /api/health/vault` — Redis ping and R2 reachability (503 when misconfigured or a dependency fails)

Status page (`/status`) reads three Better Stack Uptime monitors via the authenticated API (`BETTERSTACK_API_TOKEN` plus `BETTERSTACK_MONITOR_MARKETING`, `BETTERSTACK_MONITOR_DOCS`, `BETTERSTACK_MONITOR_VAULT`). Point those monitors at the health endpoints above. A public Better Stack status page is not required.

## Development

From the repository root:

```sh
bun install
bun run dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

The application is in `apps/web`. Run `bun run lint:web` and `bun run build:web` from the repository root before submitting changes.

## Deployment

Set the Vercel project's Root Directory to `apps/web` after connecting this monorepo. The workspace lockfile remains at the repository root.

This project uses [`next/font`](https://nextjs.org/docs/app/building-your-application/optimizing/fonts) to automatically optimize and load [Geist](https://vercel.com/font), a new font family for Vercel.
