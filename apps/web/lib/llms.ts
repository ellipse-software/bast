import { llms, type InferPageType } from "fumadocs-core/source";

import { getLatestBastVersion } from "@/lib/github";
import { defaultDescription } from "@/lib/metadata";
import {
  preWindowsInstallDescription,
  resolveWindowsReleaseContent,
  supportsWindowsRelease,
} from "@/lib/releases";
import { siteUrl } from "@/lib/site";
import { source } from "@/lib/source";

type DocPage = InferPageType<typeof source>;

export function getPageMarkdownUrl(page: DocPage) {
  const segments = [...page.slugs, "index.md"];
  return {
    segments,
    url: `/llms.mdx/docs/${segments.join("/")}`,
  };
}

export async function getLLMText(page: DocPage) {
  let processed = await page.data.getText("processed");
  if (page.url === "/docs/install") {
    processed = resolveWindowsReleaseContent(
      processed,
      supportsWindowsRelease(await getLatestBastVersion()),
    );
  }
  const pageUrl = `${siteUrl}${page.url}`;

  return `# ${page.data.title} (${pageUrl})

${processed}

---
Full docs index: ${siteUrl}/llms.txt`;
}

function absolutizeLinks(content: string): string {
  return content.replace(/\]\(\//g, `](${siteUrl}/`);
}

export async function getLlmsIndex(): Promise<string> {
  const index = llms(source, {
    renderName(node, ctx) {
      if (node.type === "page") {
        const page = source.getNodePage(node, ctx.lang);
        if (page?.data.title) return page.data.title;
      } else if (node.type !== "separator") {
        const meta = source.getNodeMeta(node, ctx.lang);
        if (meta?.data.title) return meta.data.title;
      }
      return typeof node.name === "string" ? node.name : "";
    },
  }).index();

  const preamble = `# Bast.sh

> ${defaultDescription} Docs for humans and AI agents. Use \`--json\` for scriptable CLI output. OpenSSH stays the source of truth.

## Agent resources

- [Agent skill & docs for AI](${siteUrl}/docs/reference/agents): Install for Cursor, Claude Code, and Codex
- [Skill (npx)](https://github.com/ellipse-software/bast/tree/master/skills/bast): \`npx skills add ellipse-software/bast -g -y\`
- [Skill file](${siteUrl}/bast.skill.md): curl fallback \`curl -fsSL ${siteUrl}/install-skill | sh\`
- [Full docs dump](${siteUrl}/llms-full.txt): All documentation in one file
- [GitHub (bast CLI)](https://github.com/ellipse-software/bast): Source code and releases

## Documentation

`;

  let content = `${preamble}${index.replace(/^# Documentation\n\n/, "")}`;
  if (!supportsWindowsRelease(await getLatestBastVersion())) {
    const installDescription = source.getPage(["install"])?.data.description;
    if (installDescription) {
      content = content.replace(installDescription, preWindowsInstallDescription);
    }
  }

  return absolutizeLinks(content);
}

export async function getLlmsFull(): Promise<string> {
  const pages = await Promise.all(source.getPages().map(getLLMText));
  const header = `# Bast.sh: full documentation

> ${defaultDescription}

Agent skill: npx skills add ellipse-software/bast -g -y (file: ${siteUrl}/bast.skill.md)
Docs index: ${siteUrl}/llms.txt

---

`;

  return header + pages.join("\n\n---\n\n");
}
