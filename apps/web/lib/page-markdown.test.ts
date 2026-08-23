import { describe, expect, test } from "bun:test";

import {
  markdownNotFound,
  staticMarkdownForPath,
} from "@/lib/page-markdown";
import {
  aboutMarkdown,
  contactMarkdown,
  privacyMarkdown,
} from "@/lib/trust-pages";

function visibleText(markdown: string): string {
  return markdown.replace(/[#>*`\-\[\]()]/g, " ").replace(/\s+/g, " ").trim();
}

describe("staticMarkdownForPath", () => {
  test("homepage markdown has an H1 and enough body text", () => {
    const body = staticMarkdownForPath("/");
    expect(body).toBeTruthy();
    expect(body?.startsWith("# ")).toBe(true);
    expect(body).toContain("Bast.sh");
    expect(visibleText(body ?? "").length).toBeGreaterThan(500);
  });

  test("covers trust, CLI, and developer URLs", () => {
    expect(staticMarkdownForPath("/about")).toContain("Bast.sh");
    expect(staticMarkdownForPath("/contact")).toContain("ellipse.software");
    expect(staticMarkdownForPath("/privacy")).toContain("Privacy");
    expect(staticMarkdownForPath("/cli")).toContain("brew install");
    expect(staticMarkdownForPath("/developers")).toContain("openapi.json");
    expect(staticMarkdownForPath("/termius")).toContain("Termius");
  });

  test("returns null for unknown paths", () => {
    expect(staticMarkdownForPath("/definitely-missing-xyz")).toBeNull();
  });
});

describe("trust pages", () => {
  test("about, contact, and privacy each have 500+ characters", () => {
    expect(visibleText(aboutMarkdown()).length).toBeGreaterThan(500);
    expect(visibleText(contactMarkdown()).length).toBeGreaterThan(500);
    expect(visibleText(privacyMarkdown()).length).toBeGreaterThan(500);
  });
});

describe("markdownNotFound", () => {
  test("points agents at sitemap, llms.txt, and docs", () => {
    const body = markdownNotFound("/missing-path");
    expect(body).toContain("# 404");
    expect(body).toContain("/missing-path");
    expect(body).toContain("https://bast.sh/sitemap.xml");
    expect(body).toContain("https://bast.sh/llms.txt");
    expect(body).toContain("https://bast.sh/docs");
    expect(body).toContain("https://bast.sh/openapi.json");
  });
});
