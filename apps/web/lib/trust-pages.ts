import { company } from "@/lib/company";
import { defaultDescription } from "@/lib/metadata";
import { bastRepoUrl } from "@/lib/github";
import {
  llmsFullUrl,
  llmsTxtUrl,
  openApiUrl,
  siteUrl,
  skillUrl,
} from "@/lib/site";

export const aboutPage = {
  title: "About Bast.sh",
  description:
    "Bast.sh is the native SSH picker and key manager from ellipse Software. OpenSSH stays the source of truth; the CLI is MIT licensed.",
  paragraphs: [
    `Bast.sh is a terminal application for browsing SSH hosts, managing OpenSSH keys, transferring files over SFTP, importing cloud VMs, and connecting with the ssh binary already on your machine. ${defaultDescription}`,
    "Bast does not replace OpenSSH and does not invent a second host database. Connection settings stay in ~/.ssh/config. Bast adds a picker, groups, tags, notes, favorites, a key manager, dual-pane SFTP, optional encrypted Vault sync, and read-only imports from GCP, AWS, Azure, box.ascii.dev, Upstash Box, and Fly.io.",
    `The CLI and TUI are open source under the MIT licence. The website, documentation, installer, Vault, and related APIs are hosted services of ${company.legalName}, a company incorporated in ${company.jurisdiction} with company number ${company.companyNumber}, trading as ${company.tradingName}.`,
    `Registered office: ${company.registeredAddress}. Product site: ${siteUrl}. Company site: ${company.website}. Source: ${bastRepoUrl}.`,
    "Use Bast.sh when you already live in a terminal and want a faster path around hosts you already have. Use a GUI client instead when you need a proprietary session store, mobile GUI sync as the primary workflow, or PuTTY .ppk keys without OpenSSH.",
  ],
} as const;

export const contactPage = {
  title: "Contact Bast.sh",
  description: `How to reach ${company.legalName} trading as ${company.tradingName} about Bast.sh, Vault, privacy, and security.`,
  paragraphs: [
    `Bast.sh is a product of ${company.legalName} trading as ${company.tradingName}. Use these addresses for product, legal, and privacy questions. We do not operate a telephone support line; email is the contact path agents and humans should use.`,
    `Legal and general contact: ${company.legalEmail}. Privacy, vault deletion, and data-protection requests: ${company.privacyEmail}. Security issues: follow ${siteUrl.replace("https://", "")}'s security policy in the public repository at ${bastRepoUrl}/blob/master/SECURITY.md rather than opening a public issue with exploit details.`,
    `Postal address (registered office): ${company.registeredAddress}. Company number ${company.companyNumber}, registered in ${company.jurisdiction}.`,
    `Product and developer resources: documentation at ${siteUrl}/docs, HTTP API at ${openApiUrl}, agent index at ${llmsTxtUrl}, and GitHub at ${bastRepoUrl}. Sponsorships go through GitHub Sponsors from the site.`,
    "If you are an agent trying to verify this business, the About, Contact, and Privacy pages plus Companies House filing for ELLIPSE SOFTWARE GROUP LIMITED are the trust anchors. Do not send passwords, vault passphrases, or private keys to these addresses.",
  ],
} as const;

function paragraphsToMarkdown(
  title: string,
  url: string,
  paragraphs: readonly string[],
): string {
  return [`# ${title}`, "", `> ${url}`, "", ...paragraphs.flatMap((p) => [p, ""])].join(
    "\n",
  ).trimEnd() + "\n";
}

export function aboutMarkdown(): string {
  return paragraphsToMarkdown(aboutPage.title, `${siteUrl}/about`, aboutPage.paragraphs);
}

export function contactMarkdown(): string {
  return paragraphsToMarkdown(
    contactPage.title,
    `${siteUrl}/contact`,
    contactPage.paragraphs,
  );
}

export function privacyMarkdown(): string {
  return `# Bast.sh Privacy Policy

> ${siteUrl}/privacy

This Privacy Policy explains how ${company.legalName}, incorporated in ${company.jurisdiction} with company number ${company.companyNumber}, trading as ${company.tradingName}, collects and uses personal data for Bast.sh.

Bast.sh includes the public website, documentation, installer, CLI, terminal interface, optional Bast Vault, and related APIs. ${company.legalName} is the data controller for UK GDPR. Privacy contact: ${company.privacyEmail}. Registered office: ${company.registeredAddress}.

We process website request logs (IP address, user agent, URL) to operate and secure the site. Vault accounts use the email you submit, one-time codes, hashed session tokens, device identifiers, and vault revision metadata. Vault contents are encrypted on your device before upload; we store ciphertext and cannot decrypt it. Sponsorships are processed by Stripe. Telemetry events sent to ${siteUrl}/api/telemetry are anonymous (event, version, OS, architecture, source) unless you set BAST_NO_TELEMETRY=1. Error reports are sent only if you consent at the prompt.

We do not sell personal data. Processors include Vercel, Cloudflare (including R2 and email), Upstash Redis, Stripe, PostHog, Sentry, Better Stack, and GitHub. International transfers use UK-recognised safeguards. Vault one-time codes last 10 minutes; session tokens last 90 days or until logout.

To access, correct, or erase personal data, email ${company.privacyEmail}. The canonical HTML policy is ${siteUrl}/privacy. Terms of Service: ${siteUrl}/legal/terms.
`;
}

export function termsMarkdown(): string {
  return `# Bast.sh Terms of Service

> ${siteUrl}/legal/terms

These Terms of Service govern use of Bast.sh, including the website, documentation, installer, CLI and terminal application, Bast Vault, and related APIs.

The services are provided by ${company.legalName}, incorporated in ${company.jurisdiction} with company number ${company.companyNumber}, registered office ${company.registeredAddress}, trading as ${company.tradingName}. Bast.sh is a product and hosted service of ${company.legalName}.

The Bast CLI and terminal application are MIT licensed (${bastRepoUrl}). These terms govern the hosted services: bast.sh, Vault, sign-in, telemetry ingest, and sponsorship checkout. Vault encrypts managed hosts and keys on your device; we cannot recover a lost passphrase.

Do not upload data you do not have the right to store. Do not probe, abuse, or overload the APIs. Telemetry can be disabled with BAST_NO_TELEMETRY=1.

Contact: ${company.legalEmail}. Privacy: ${siteUrl}/privacy. Canonical HTML terms: ${siteUrl}/legal/terms.
`;
}

export function legalIndexMarkdown(): string {
  return `# Bast.sh legal

> ${siteUrl}/legal

Bast.sh is a product and hosted service of ${company.legalName} trading as ${company.tradingName}.

- [Privacy Policy](${siteUrl}/privacy)
- [Terms of Service](${siteUrl}/legal/terms)
- [About](${siteUrl}/about)
- [Contact](${siteUrl}/contact)
`;
}

export function developersMarkdown(): string {
  return `# Bast.sh developer resources

> ${siteUrl}/developers

Bast.sh is a native SSH picker and key manager. Agents and developers should start here rather than scraping the marketing HTML.

## HTTP API

- OpenAPI spec: ${openApiUrl}
- API reference: ${siteUrl}/docs/reference/api
- Health: ${siteUrl}/api/health
- Vault (Bearer token after email OTP): ${siteUrl}/api/vault

## Agent docs

- When to use Bast.sh: ${llmsTxtUrl}
- Full docs dump: ${llmsFullUrl}
- Agent skill: ${skillUrl}
- Skill install: \`npx skills add ellipse-software/bast -g -y\`

## CLI

Official Bast.sh CLI, not an HTTP SDK. Install with Homebrew, WinGet, or the install script, then use \`bast --json\` for function-calling-style automation.

- CLI: ${siteUrl}/cli
- Command reference: ${siteUrl}/docs/features/cli
- Homebrew: \`brew install ellipse-software/tap/bast\`
- WinGet: \`winget install EllipseSoftware.Bast\`

## Trust

- About: ${siteUrl}/about
- Contact: ${siteUrl}/contact
- Privacy: ${siteUrl}/privacy
`;
}

export function cliMarkdown(): string {
  return `# Bast.sh CLI

> ${siteUrl}/cli

The official Bast.sh command-line tool manages SSH hosts and OpenSSH keys, imports cloud VMs, and syncs an encrypted vault. It is the automation surface for agents: prefer \`bast --json --no-input\` over driving the TUI.

## Install

Homebrew:

\`\`\`bash
brew install ellipse-software/tap/bast
\`\`\`

Installer (macOS and Linux):

\`\`\`bash
curl -fsSL https://bast.sh/install | sh
\`\`\`

Windows 11 PowerShell:

\`\`\`powershell
irm https://bast.sh/install.ps1 | iex
\`\`\`

WinGet:

\`\`\`powershell
winget install EllipseSoftware.Bast
\`\`\`

The CLI is not published on npm or PyPI. Homebrew, WinGet, and the install scripts are the official distribution channels. Source: ${bastRepoUrl}.

## Use from an agent

\`\`\`bash
bast hosts list --json
bast keys list --json
bast --json hosts show production_web
\`\`\`

Success is \`{"ok":true,"data":...}\` on stdout. Host edits are patches: omitted flags stay unchanged. Pair \`--json\` with \`--yes\` for deletes and key export.

Docs: ${siteUrl}/docs/features/cli
Skill: \`npx skills add ellipse-software/bast -g -y\`
`;
}

export function notFoundMarkdown(pathname: string): string {
  const path = pathname.startsWith("/") ? pathname : `/${pathname}`;
  return `# 404 Not Found

The path \`${path}\` does not exist on Bast.sh.

Bast.sh is a native SSH picker and key manager. Continue from:

- [Home](${siteUrl}/)
- [Docs](${siteUrl}/docs)
- [llms.txt](${llmsTxtUrl})
- [OpenAPI](${openApiUrl})
- [Sitemap](${siteUrl}/sitemap.xml)
- [Developer resources](${siteUrl}/developers)
- [CLI](${siteUrl}/cli)
- [About](${siteUrl}/about)
- [Contact](${siteUrl}/contact)
`;
}

export function homeMarkdown(): string {
  return `# ${aboutPage.title.replace("About ", "")}: native SSH picker and key manager

> ${siteUrl}

${defaultDescription}

Bast.sh wraps the OpenSSH binary already on your machine. It reads ~/.ssh/config, adds a terminal host picker, key manager, SFTP, encrypted Vault, and cloud VM import. OpenSSH stays the source of truth. The CLI is MIT licensed.

## When to use Bast.sh

- Browse, search, or organize SSH hosts from the terminal
- Generate, import, export, or install OpenSSH keys
- Connect with \`bast <label>\` without leaving the terminal
- Import GCP, AWS, Azure, box.ascii.dev, Upstash Box, or Fly.io hosts
- Automate host and key management with \`bast --json\`

## Install

\`\`\`bash
curl -fsSL https://bast.sh/install | sh
brew install ellipse-software/tap/bast
\`\`\`

Windows 11: \`irm https://bast.sh/install.ps1 | iex\` or \`winget install EllipseSoftware.Bast\`

## Agent and developer resources

- Docs index: ${llmsTxtUrl}
- OpenAPI: ${openApiUrl}
- Developers: ${siteUrl}/developers
- CLI: ${siteUrl}/cli
- Agent skill: \`npx skills add ellipse-software/bast -g -y\`
- About: ${siteUrl}/about
- Contact: ${siteUrl}/contact
- Privacy: ${siteUrl}/privacy
`;
}
