import { describe, expect, test } from "bun:test";

import {
  appendVaryAccept,
  markdownResponseHeaders,
  markdownRewritePath,
  preferredType,
  shouldNegotiateMarkdown,
} from "@/lib/accept";

describe("preferredType", () => {
  test("defaults to HTML when Accept is missing", () => {
    expect(preferredType(null)).toBe("text/html");
    expect(preferredType("")).toBe("text/html");
  });

  test("selects markdown when it is the only type", () => {
    expect(preferredType("text/markdown")).toBe("text/markdown");
  });

  test("selects markdown when it appears before HTML at the same q", () => {
    expect(preferredType("text/markdown, text/html, */*")).toBe("text/markdown");
  });

  test("selects HTML when the browser lists it first", () => {
    expect(
      preferredType(
        "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
      ),
    ).toBe("text/html");
  });

  test("honors q-values so HTML can beat markdown", () => {
    expect(preferredType("text/markdown;q=0.1, text/html;q=0.9")).toBe(
      "text/html",
    );
  });

  test("rejects HTML when q=0 even if a wildcard is present", () => {
    expect(preferredType("text/html;q=0, */*;q=1")).toBe("text/markdown");
  });

  test("returns null when every produced type is rejected", () => {
    expect(preferredType("application/pdf")).toBe(null);
    expect(preferredType("text/html;q=0, text/markdown;q=0, */*;q=0")).toBe(
      null,
    );
  });
});

describe("appendVaryAccept", () => {
  test("sets Vary: Accept when missing", () => {
    const headers = new Headers();
    appendVaryAccept(headers);
    expect(headers.get("Vary")).toBe("Accept");
  });

  test("appends Accept without duplicating it", () => {
    const headers = new Headers({
      Vary: "rsc, next-router-state-tree, accept-encoding",
    });
    appendVaryAccept(headers);
    appendVaryAccept(headers);
    expect(headers.get("Vary")).toBe(
      "rsc, next-router-state-tree, accept-encoding, Accept",
    );
  });
});

describe("shouldNegotiateMarkdown", () => {
  test("negotiates marketing and docs paths", () => {
    expect(shouldNegotiateMarkdown("/")).toBe(true);
    expect(shouldNegotiateMarkdown("/docs/install")).toBe(true);
    expect(shouldNegotiateMarkdown("/about")).toBe(true);
  });

  test("skips APIs, assets, and already-markdown files", () => {
    expect(shouldNegotiateMarkdown("/api/health")).toBe(false);
    expect(shouldNegotiateMarkdown("/openapi.json")).toBe(false);
    expect(shouldNegotiateMarkdown("/llms.txt")).toBe(false);
    expect(shouldNegotiateMarkdown("/install")).toBe(false);
    expect(shouldNegotiateMarkdown("/markdown/docs")).toBe(false);
    expect(shouldNegotiateMarkdown("/bast.skill.md")).toBe(false);
  });
});

describe("markdownRewritePath", () => {
  test("maps the homepage and nested paths", () => {
    expect(markdownRewritePath("/")).toBe("/markdown");
    expect(markdownRewritePath("/docs/install")).toBe("/markdown/docs/install");
  });
});

describe("markdownResponseHeaders", () => {
  test("advertises markdown and Vary: Accept", () => {
    const headers = new Headers(markdownResponseHeaders());
    expect(headers.get("Content-Type")).toBe("text/markdown; charset=utf-8");
    expect(headers.get("Vary")?.toLowerCase()).toContain("accept");
  });
});
