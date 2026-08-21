import { describe, expect, test } from "bun:test";

import { isAllowedAvatarUrl } from "@/lib/avatar";
import { normalizeXHandle, parseXProfileUrl } from "@/lib/x-handle";

describe("normalizeXHandle", () => {
  test("accepts handles with or without @", () => {
    expect(normalizeXHandle("jack")).toBe("jack");
    expect(normalizeXHandle("@Jack")).toBe("jack");
    expect(normalizeXHandle("  @elon_musk ")).toBe("elon_musk");
  });

  test("rejects invalid handles", () => {
    expect(normalizeXHandle("")).toBeNull();
    expect(normalizeXHandle("@")).toBeNull();
    expect(normalizeXHandle("thisnameistoolong")).toBeNull();
    expect(normalizeXHandle("bad-name")).toBeNull();
    expect(normalizeXHandle("bad.name")).toBeNull();
    expect(normalizeXHandle("../etc")).toBeNull();
  });
});

describe("parseXProfileUrl", () => {
  test("reads x.com and twitter.com profile URLs", () => {
    expect(parseXProfileUrl("https://x.com/jess_daniel10")).toBe("jess_daniel10");
    expect(parseXProfileUrl("https://twitter.com/@Jess_Daniel10/")).toBe(
      "jess_daniel10",
    );
    expect(parseXProfileUrl("https://www.x.com/maxktz?s=20")).toBe("maxktz");
  });

  test("rejects status URLs and reserved paths", () => {
    expect(
      parseXProfileUrl("https://x.com/jess_daniel10/status/2087927680796614820"),
    ).toBeNull();
    expect(parseXProfileUrl("https://x.com/home")).toBeNull();
    expect(parseXProfileUrl("https://example.com/jess_daniel10")).toBeNull();
  });
});

describe("isAllowedAvatarUrl", () => {
  test("allows X and unavatar HTTPS hosts", () => {
    expect(
      isAllowedAvatarUrl("https://pbs.twimg.com/profile_images/1.jpg"),
    ).toBe(true);
    expect(isAllowedAvatarUrl("https://unavatar.io/x/jack")).toBe(true);
  });

  test("rejects other hosts and protocols", () => {
    expect(isAllowedAvatarUrl("http://pbs.twimg.com/1.jpg")).toBe(false);
    expect(isAllowedAvatarUrl("https://evil.example/1.jpg")).toBe(false);
    expect(isAllowedAvatarUrl("not a url")).toBe(false);
  });
});
