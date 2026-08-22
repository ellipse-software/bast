import { describe, expect, test } from "bun:test";
import { remark } from "remark";

import { remarkGfm } from "fumadocs-core/mdx-plugins/remark-gfm";

import {
  linkifyGithubText,
  remarkChangelogGithub,
  shortenBastGithubUrl,
} from "@/lib/changelog-markdown";

function toMarkdown(input: string): string {
  return String(
    remark().use(remarkGfm).use(remarkChangelogGithub).processSync(input),
  ).trim();
}

describe("shortenBastGithubUrl", () => {
  test("shortens pull and issue URLs to hash references", () => {
    expect(
      shortenBastGithubUrl("https://github.com/ellipse-software/bast/pull/40"),
    ).toBe("#40");
    expect(
      shortenBastGithubUrl(
        "https://github.com/ellipse-software/bast/issues/18",
      ),
    ).toBe("#18");
  });

  test("strips the repo prefix from other GitHub paths", () => {
    expect(
      shortenBastGithubUrl(
        "https://github.com/ellipse-software/bast/compare/v0.6.5...v0.6.6",
      ),
    ).toBe("compare/v0.6.5...v0.6.6");
  });

  test("ignores other hosts and the repo root", () => {
    expect(shortenBastGithubUrl("https://example.com/pull/1")).toBeNull();
    expect(
      shortenBastGithubUrl("https://github.com/ellipse-software/bast"),
    ).toBeNull();
    expect(
      shortenBastGithubUrl("https://github.com/ellipse-software/bast/"),
    ).toBeNull();
  });
});

describe("linkifyGithubText", () => {
  test("links mentions, bots, and issue numbers", () => {
    expect(
      linkifyGithubText("by @kevinpita and @dependabot[bot] in #40"),
    ).toEqual([
      { type: "text", value: "by " },
      {
        type: "link",
        value: "@kevinpita",
        url: "https://github.com/kevinpita",
      },
      { type: "text", value: " and " },
      {
        type: "link",
        value: "@dependabot[bot]",
        url: "https://github.com/apps/dependabot",
      },
      { type: "text", value: " in " },
      {
        type: "link",
        value: "#40",
        url: "https://github.com/ellipse-software/bast/issues/40",
      },
    ]);
  });

  test("does not treat emails as mentions", () => {
    expect(linkifyGithubText("write ted@example.com please")).toEqual([
      { type: "text", value: "write ted@example.com please" },
    ]);
  });
});

describe("remarkChangelogGithub", () => {
  test("renders GitHub release markdown with shortened repo links", () => {
    const markdown = toMarkdown(
      "## What's Changed\n* feat: import hosts from history by @tedbrine in https://github.com/ellipse-software/bast/pull/31\n\n**Full Changelog**: https://github.com/ellipse-software/bast/compare/v0.6.3...v0.6.4",
    );

    expect(markdown).toContain("What's Changed");
    expect(markdown).toContain("[@tedbrine](https://github.com/tedbrine)");
    expect(markdown).toContain(
      "[#31](https://github.com/ellipse-software/bast/pull/31)",
    );
    expect(markdown).toContain(
      "[compare/v0.6.3...v0.6.4](https://github.com/ellipse-software/bast/compare/v0.6.3...v0.6.4)",
    );
    expect(markdown).toContain("**Full Changelog**");
  });

  test("does not rewrite custom link text or code spans", () => {
    const markdown = toMarkdown(
      "See [the PR](https://github.com/ellipse-software/bast/pull/31) and `#32` plus `@nobody`.",
    );

    expect(markdown).toContain(
      "[the PR](https://github.com/ellipse-software/bast/pull/31)",
    );
    expect(markdown).toContain("`#32`");
    expect(markdown).toContain("`@nobody`");
  });
});
