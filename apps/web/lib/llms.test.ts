import { describe, expect, test } from "bun:test";

import { getLlmsPreamble } from "@/lib/llms-preamble";

describe("getLlmsPreamble", () => {
  test("tells agents when to use Bast.sh and how to call it", () => {
    const text = getLlmsPreamble();
    expect(text.startsWith("# Bast.sh")).toBe(true);
    expect(text).toContain("## When to use Bast.sh");
    expect(text).toContain("bast --json");
    expect(text).toContain("brew install ellipse-software/tap/bast");
    expect(text).toContain("Do not use Bast.sh");
    expect(text).toContain("https://bast.sh/openapi.json");
    expect(text).toContain("https://bast.sh/developers");
    expect(text).toContain("https://bast.sh/cli");
  });
});
