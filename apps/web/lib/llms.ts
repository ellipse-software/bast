import { llms, type InferPageType } from "fumadocs-core/source";

import { getLatestBastVersion } from "@/lib/github";
import { getLlmsPreamble } from "@/lib/llms-preamble";
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

export { getLlmsPreamble } from "@/lib/llms-preamble";

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

  let content = `${getLlmsPreamble()}${index.replace(/^# Documentation\n\n/, "")}`;
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

When to use Bast.sh, the CLI, and the hosted API: ${siteUrl}/llms.txt
OpenAPI: ${siteUrl}/openapi.json
Agent skill: npx skills add ellipse-software/bast -g -y (file: ${siteUrl}/bast.skill.md)

---

`;

  return header + pages.join("\n\n---\n\n");
}
