import Link from "next/link";
import type { ReactNode } from "react";

import { AgentResources } from "@/components/ask-ai-menu";
import { FooterWordmark } from "@/components/footer-wordmark";
import { WordmarkLogo } from "@/components/wordmark-logo";
import {
  aggregateLabel,
  getStatusPageData,
  type AggregateStatus,
} from "@/lib/betterstack";
import { bastRepoUrl, bastReleasesUrl, bastSponsorUrl } from "@/lib/github";
import {
  comparisonNavItems,
  guideNavItems,
} from "@/lib/marketing";
import { llmsFullUrl, llmsTxtUrl, skillUrl } from "@/lib/site";
import { pageMaxWidthClass } from "@/lib/layout";

const linkClass =
  "text-sm text-muted transition-colors hover:text-foreground";

function FooterColumn({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <div>
      <h2 className="mb-3 text-sm font-medium tracking-tight text-foreground">
        {title}
      </h2>
      <ul className="space-y-2">{children}</ul>
    </div>
  );
}

function FooterLink({
  href,
  children,
  external = false,
}: {
  href: string;
  children: ReactNode;
  external?: boolean;
}) {
  if (external) {
    return (
      <li>
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className={linkClass}
        >
          {children}
        </a>
      </li>
    );
  }

  return (
    <li>
      <Link href={href} className={linkClass}>
        {children}
      </Link>
    </li>
  );
}

function statusDotClass(status: AggregateStatus): string {
  switch (status) {
    case "operational":
      return "bg-[#3fb950]";
    case "downtime":
      return "bg-[#f85149]";
    case "maintenance":
    case "degraded":
      return "bg-[#d4a72c]";
    case "unknown":
      return "bg-muted";
  }
}

function footerStatusLabel(status: AggregateStatus): string {
  switch (status) {
    case "operational":
      return "Systems operational";
    case "downtime":
      return "Major outage";
    case "maintenance":
      return "Under maintenance";
    case "degraded":
      return "Partial outage";
    case "unknown":
      return "Status";
  }
}

export async function SiteFooter() {
  const status = await getStatusPageData();

  return (
    <footer className="relative z-10 border-t border-border bg-background">
      <div
        className={`mx-auto w-full ${pageMaxWidthClass} px-4 py-12 sm:px-6 sm:py-14`}
      >
        <div className="mb-10 flex flex-col gap-6 sm:mb-12 sm:flex-row sm:items-start sm:justify-between">
          <div className="max-w-sm">
            <Link href="/" className="inline-flex">
              <WordmarkLogo />
            </Link>
            <p className="mt-3 text-sm leading-relaxed text-muted">
              Terminal-native SSH host picker for macOS and Linux. OpenSSH stays
              the source of truth.
            </p>
          </div>
          <AgentResources contextUrl={llmsTxtUrl} />
        </div>

        <div className="grid grid-cols-2 gap-8 sm:grid-cols-4">
          <FooterColumn title="Features">
            <FooterLink href="/features">Overview</FooterLink>
            {guideNavItems.map((item) => (
              <FooterLink key={item.href} href={item.href}>
                {item.label}
              </FooterLink>
            ))}
          </FooterColumn>

          <FooterColumn title="Compare">
            <FooterLink href="/alternatives">All comparisons</FooterLink>
            {comparisonNavItems.map((item) => (
              <FooterLink key={item.href} href={item.href}>
                vs {item.label}
              </FooterLink>
            ))}
          </FooterColumn>

          <FooterColumn title="Product">
            <FooterLink href="/docs">Docs</FooterLink>
            <FooterLink href="/changelog">Changelog</FooterLink>
            <FooterLink href="/status">Status</FooterLink>
            <FooterLink href="/docs/install">Install</FooterLink>
            <FooterLink href="/docs/features/cli">CLI</FooterLink>
            <FooterLink href="/docs/features/vault">Vault</FooterLink>
          </FooterColumn>

          <FooterColumn title="Resources">
            <FooterLink href={bastRepoUrl} external>
              GitHub
            </FooterLink>
            <FooterLink href={bastReleasesUrl} external>
              Releases
            </FooterLink>
            <FooterLink href={bastSponsorUrl} external>
              Sponsor
            </FooterLink>
            <FooterLink href={llmsTxtUrl}>llms.txt</FooterLink>
            <FooterLink href={llmsFullUrl}>llms-full.txt</FooterLink>
            <FooterLink href={skillUrl}>Agent skill</FooterLink>
          </FooterColumn>
        </div>

        <div className="mt-12 flex flex-col gap-3 border-t border-border pt-6 text-xs text-muted sm:flex-row sm:items-center sm:justify-between">
          <p>
            © {new Date().getFullYear()} Bast.sh
            <span aria-hidden className="mx-2 text-border">
              ·
            </span>
            An{" "}
            <a
              href="https://ellipse.software"
              target="_blank"
              rel="noopener noreferrer"
              className="transition-colors hover:text-foreground"
            >
              ellipse Software
            </a>{" "}
            product.
          </p>
          <p className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <Link
              href="/status"
              className="inline-flex items-center gap-1.5 transition-colors hover:text-foreground"
              title={aggregateLabel(status.aggregate)}
            >
              <span
                className={`size-1.5 shrink-0 rounded-full ${statusDotClass(status.aggregate)}`}
                aria-hidden
              />
              {footerStatusLabel(status.aggregate)}
            </Link>
            <span aria-hidden className="text-border">
              ·
            </span>
            <span>MIT licensed</span>
          </p>
        </div>
      </div>

      <FooterWordmark />
    </footer>
  );
}
