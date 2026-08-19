import { describe, expect, test } from "bun:test";

import {
  resolveWindowsReleaseContent,
  supportsWindowsRelease,
} from "@/lib/releases";

describe("supportsWindowsRelease", () => {
  const cases: Array<[string | null, boolean]> = [
    [null, false],
    ["v0.8.1", false],
    ["v0.9.0", true],
    ["0.10.0", true],
    ["v1.0.0", true],
    ["v0.9.0-rc.1", false],
    ["nightly.20260819.abcdef0", false],
  ];

  for (const [version, expected] of cases) {
    test(`${version ?? "null"} -> ${expected}`, () => {
      expect(supportsWindowsRelease(version)).toBe(expected);
    });
  }
});

describe("resolveWindowsReleaseContent", () => {
  const content = `Before

<WindowsReleaseOnly>
Windows instructions
</WindowsReleaseOnly>

After`;

  test("removes unreleased Windows content", () => {
    expect(resolveWindowsReleaseContent(content, false)).not.toContain(
      "Windows instructions",
    );
  });

  test("keeps released Windows content without the MDX wrapper", () => {
    const resolved = resolveWindowsReleaseContent(content, true);
    expect(resolved).toContain("Windows instructions");
    expect(resolved).not.toContain("WindowsReleaseOnly");
  });
});
