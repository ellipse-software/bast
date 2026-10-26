import Link from "next/link";

import { MarketingBreadcrumb, MarketingShell } from "@/components/marketing-shell";
import { developersPath } from "@/lib/company";
import { getLatestBastVersion } from "@/lib/github";
import { createPageMetadata } from "@/lib/metadata";
import { llmsFullUrl, llmsTxtUrl, openApiUrl, skillUrl } from "@/lib/site";
import { legalLinkClass } from "@/components/legal-page";

export const metadata = createPageMetadata({
  title: "Bast.sh developer resources",
  description:
    "Bast.sh OpenAPI spec, HTTP API, CLI, agent skill, and llms.txt. How agents should call Bast.sh.",
  path: developersPath,
});

const resources = [
  {
    href: openApiUrl,
    title: "Bast.sh OpenAPI spec",
    blurb: "Machine-readable HTTP API at /openapi.json.",
    external: true,
  },
  {
    href: "/docs/reference/api",
    title: "Bast.sh HTTP API",
    blurb: "Vault, health, search, telemetry, and JSON errors.",
  },
  {
    href: "/cli",
    title: "Bast.sh CLI",
    blurb: "Homebrew, Linux packages, WinGet, and bast --json automation.",
  },
  {
    href: "/docs/reference/agents",
    title: "Bast.sh agent skill",
    blurb: "Install for Cursor, Claude Code, and Codex.",
  },
  {
    href: llmsTxtUrl,
    title: "llms.txt",
    blurb: "When to use Bast.sh, plus the docs index.",
    external: true,
  },
  {
    href: llmsFullUrl,
    title: "llms-full.txt",
    blurb: "All documentation in one file.",
    external: true,
  },
  {
    href: skillUrl,
    title: "SKILL.md",
    blurb: "Raw skill file for curl installs.",
    external: true,
  },
  {
    href: "/docs/features/vault",
    title: "Bast Vault",
    blurb: "End-to-end encrypted host and key sync.",
  },
] as const;

export default async function DevelopersPage() {
  const version = await getLatestBastVersion();
  return (
    <MarketingShell version={version}>
      <MarketingBreadcrumb label="Developers" />
      <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
        Bast.sh developer resources
      </h1>
      <p className="mb-10 max-w-2xl text-base leading-relaxed text-muted sm:text-lg">
        Bast.sh is a native SSH picker and key manager. Local host and key
        automation uses the CLI. The hosted HTTP API is Vault, health, docs
        search, and telemetry.
      </p>
      <div className="bg-border p-px">
        <div className="divide-y divide-border bg-background">
          {resources.map((item) => {
            const className =
              "block px-4 py-4 transition-colors hover:bg-surface sm:px-5";
            const body = (
              <>
                <h2 className="mb-1 text-base font-medium tracking-tight text-foreground">
                  {item.title}
                </h2>
                <p className="text-sm leading-relaxed text-muted">{item.blurb}</p>
              </>
            );
            if ("external" in item && item.external) {
              return (
                <a
                  key={item.href}
                  href={item.href}
                  className={className}
                  rel={item.href.startsWith("https://bast.sh") ? undefined : "noopener noreferrer"}
                >
                  {body}
                </a>
              );
            }
            return (
              <Link key={item.href} href={item.href} className={className}>
                {body}
              </Link>
            );
          })}
        </div>
      </div>
      <p className="mt-8 text-sm text-muted">
        OpenAPI:{" "}
        <a href={openApiUrl} className={legalLinkClass}>
          {openApiUrl}
        </a>
      </p>
    </MarketingShell>
  );
}
