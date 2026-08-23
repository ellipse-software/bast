import { mobaxtermComparison } from "@/lib/comparisons/mobaxterm";
import { puttyComparison } from "@/lib/comparisons/putty";
import { securecrtComparison } from "@/lib/comparisons/securecrt";
import { termiusComparison } from "@/lib/comparisons/termius";
import type { ComparisonCaseStudy } from "@/lib/comparisons/types";
import { cloudSshGuide } from "@/lib/guides/cloud-ssh";
import { sshHostManagerGuide } from "@/lib/guides/ssh-host-manager";
import { sshKeyManagerGuide } from "@/lib/guides/ssh-key-manager";
import { sshSftpGuide } from "@/lib/guides/ssh-sftp";
import { syncSshHostsGuide } from "@/lib/guides/sync-ssh-hosts";
import type { GuidePage } from "@/lib/guides/types";
import { homeHeadline, homeLead, homeSecondary } from "@/lib/home";
import { comparisonNavItems, guideNavItems } from "@/lib/marketing";
import { defaultDescription } from "@/lib/metadata";
import { siteUrl } from "@/lib/site";
import {
  aboutMarkdown,
  cliMarkdown,
  contactMarkdown,
  developersMarkdown,
  homeMarkdown,
  legalIndexMarkdown,
  notFoundMarkdown,
  privacyMarkdown,
  termsMarkdown,
} from "@/lib/trust-pages";

function normalizePath(pathname: string): string {
  if (!pathname || pathname === "/") return "/";
  const withSlash = pathname.startsWith("/") ? pathname : `/${pathname}`;
  return withSlash.length > 1 && withSlash.endsWith("/")
    ? withSlash.slice(0, -1)
    : withSlash;
}

function comparisonMarkdown(content: ComparisonCaseStudy): string {
  const rows = content.diffRows
    .map((row) => `| ${row.topic} | ${row.bast} | ${row.competitor} |`)
    .join("\n");
  const faqs = content.faqs
    .map((item) => `### ${item.q}\n\n${item.a}`)
    .join("\n\n");
  const better = content.whenBetterItems.map((item) => `- ${item}`).join("\n");
  return `# ${content.title}

> ${siteUrl}/${content.slug}

${content.description}

${content.lead}

## Differences

| Topic | Bast.sh | ${content.competitorName} |
| --- | --- | --- |
${rows}

## ${content.whenBetterTitle}

${content.whenBetterIntro}

${better}

${content.whenBetterOutro}

## FAQ

${faqs}

Full HTML page: ${siteUrl}/${content.slug}
Related docs: ${siteUrl}/docs
`;
}

function guideMarkdown(content: GuidePage): string {
  return `# ${content.title}

> ${siteUrl}/${content.slug}

${content.description}

${content.lead}

${content.problemTitle}. ${content.solutionTitle}.

Install Bast.sh, then follow the HTML guide at ${siteUrl}/${content.slug} or the docs index at ${siteUrl}/llms.txt.
`;
}

const comparisons: Record<string, ComparisonCaseStudy> = {
  "/termius": termiusComparison,
  "/putty": puttyComparison,
  "/mobaxterm": mobaxtermComparison,
  "/securecrt": securecrtComparison,
};

const guides: Record<string, GuidePage> = {
  "/ssh-host-manager": sshHostManagerGuide,
  "/sync-ssh-hosts": syncSshHostsGuide,
  "/cloud-ssh": cloudSshGuide,
  "/ssh-sftp": sshSftpGuide,
  "/ssh-key-manager": sshKeyManagerGuide,
};

function featuresMarkdown(): string {
  const items = guideNavItems
    .map((item) => `- [${item.label}](${siteUrl}${item.href}): ${item.blurb}`)
    .join("\n");
  return `# Bast.sh features

> ${siteUrl}/features

Bast stays in the terminal and keeps OpenSSH in charge.

${items}

Docs: ${siteUrl}/docs
`;
}

function alternativesMarkdown(): string {
  const items = comparisonNavItems
    .map((item) => `- [Bast vs ${item.label}](${siteUrl}${item.href}): ${item.blurb}`)
    .join("\n");
  return `# Bast.sh alternatives and comparisons

> ${siteUrl}/alternatives

${defaultDescription}

${items}
`;
}

function changelogMarkdown(): string {
  return `# Bast.sh changelog

> ${siteUrl}/changelog

Stable Bast.sh releases are published on GitHub. The HTML page lists notes for each version.

- Releases: https://github.com/ellipse-software/bast/releases
- Install the latest CLI: ${siteUrl}/cli
`;
}

function statusMarkdown(): string {
  return `# Bast.sh status

> ${siteUrl}/status

Live availability for the Bast.sh marketing site, docs, and Vault.

JSON health endpoints:

- ${siteUrl}/api/health
- ${siteUrl}/api/health/docs
- ${siteUrl}/api/health/vault
`;
}

const STATIC_PAGES: Record<string, () => string> = {
  "/": homeMarkdown,
  "/about": aboutMarkdown,
  "/contact": contactMarkdown,
  "/privacy": privacyMarkdown,
  "/legal/privacy": privacyMarkdown,
  "/legal/terms": termsMarkdown,
  "/legal": legalIndexMarkdown,
  "/cli": cliMarkdown,
  "/developers": developersMarkdown,
  "/features": featuresMarkdown,
  "/alternatives": alternativesMarkdown,
  "/changelog": changelogMarkdown,
  "/status": statusMarkdown,
};

export function staticMarkdownForPath(pathname: string): string | null {
  const path = normalizePath(pathname);
  const staticPage = STATIC_PAGES[path];
  if (staticPage) return staticPage();
  const comparison = comparisons[path];
  if (comparison) return comparisonMarkdown(comparison);
  const guide = guides[path];
  if (guide) return guideMarkdown(guide);
  return null;
}

export function markdownNotFound(pathname: string): string {
  return notFoundMarkdown(normalizePath(pathname));
}

export { homeHeadline, homeLead, homeSecondary };
