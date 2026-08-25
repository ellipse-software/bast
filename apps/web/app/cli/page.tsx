import Link from "next/link";

import { InstallCommand } from "@/components/install-command";
import { MarketingBreadcrumb, MarketingShell } from "@/components/marketing-shell";
import { cliPath } from "@/lib/company";
import { winget } from "@/flags";
import { getLatestBastVersion } from "@/lib/github";
import { createPageMetadata } from "@/lib/metadata";
import { legalLinkClass } from "@/components/legal-page";

export const metadata = createPageMetadata({
  title: "Bast.sh CLI",
  description:
    "Install the official Bast.sh CLI with Homebrew, Linux packages, WinGet, or the install script. Automate SSH hosts and keys with bast --json.",
  path: cliPath,
});

const methods = [
  {
    label: "Homebrew",
    command: "brew install ellipse-software/tap/bast",
  },
  {
    label: "Linux packages",
    command: "curl -fsSL https://packages.bast.sh/setup.sh | sudo sh",
  },
  {
    label: "Installer",
    command: "curl -fsSL https://bast.sh/install | sh",
  },
  {
    label: "Windows 11 PowerShell",
    command: "irm https://bast.sh/install.ps1 | iex",
  },
  {
    label: "WinGet",
    command: "winget install EllipseSoftware.Bast",
  },
] as const;

export default async function CliPage() {
  const [version, wingetAvailable] = await Promise.all([
    getLatestBastVersion(),
    winget(),
  ]);
  return (
    <MarketingShell version={version}>
      <MarketingBreadcrumb label="CLI" />
      <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
        Bast.sh CLI
      </h1>
      <p className="mb-10 max-w-2xl text-base leading-relaxed text-muted sm:text-lg">
        The official Bast.sh command-line tool for macOS, Linux, and Windows 11.
        It is published via Homebrew, Linux packages, WinGet, and the bast.sh
        install scripts. Agents should call{" "}
        <code className="text-foreground">bast --json</code> rather than driving
        the TUI.
      </p>

      <section className="mb-12 max-w-2xl space-y-4 text-sm leading-relaxed text-muted sm:text-[15px]">
        <h2 className="text-xl font-medium tracking-tight text-foreground">
          Install
        </h2>
        <p>
          Homebrew is the named package:{" "}
          <code className="text-foreground">ellipse-software/tap/bast</code>.
          Linux packages install <code className="text-foreground">bast</code>{" "}
          from <code className="text-foreground">packages.bast.sh</code>.
          WinGet package id{" "}
          <code className="text-foreground">EllipseSoftware.Bast</code>. The
          CLI is not published on npm or PyPI.
        </p>
        <ul className="space-y-2 font-mono text-xs text-foreground sm:text-sm">
          {methods.map((method) => (
            <li key={method.label}>
              <span className="block text-[11px] uppercase tracking-widest text-muted">
                {method.label}
              </span>
              <code>{method.command}</code>
            </li>
          ))}
        </ul>
        <InstallCommand
          version={version}
          wingetAvailable={wingetAvailable}
          className="w-full"
        />
      </section>

      <section className="mb-12 max-w-2xl space-y-4 text-sm leading-relaxed text-muted sm:text-[15px]">
        <h2 className="text-xl font-medium tracking-tight text-foreground">
          Agent usage
        </h2>
        <p>
          Pair <code className="text-foreground">--json</code> with explicit
          flags. Host edits are patches. Use{" "}
          <code className="text-foreground">--yes</code> for deletes and key
          export. Install the skill with{" "}
          <code className="text-foreground">
            npx skills add ellipse-software/bast -g -y
          </code>
          .
        </p>
        <pre className="overflow-x-auto border border-border bg-surface px-4 py-3 font-mono text-xs text-foreground">
          {`bast hosts list --json
bast keys list --json
bast doctor --json
bast --json hosts show production_web`}
        </pre>
        <p>
          Command reference:{" "}
          <Link href="/docs/features/cli" className={legalLinkClass}>
            Command line
          </Link>
          {" · "}
          <Link href="/docs/features/doctor" className={legalLinkClass}>
            Doctor
          </Link>
          . HTTP API for Vault lives at{" "}
          <Link href="/developers" className={legalLinkClass}>
            developer resources
          </Link>
          .
        </p>
      </section>
    </MarketingShell>
  );
}
