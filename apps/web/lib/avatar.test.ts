import { describe, expect, test } from "bun:test";

import { isAllowedAvatarUrl } from "@/lib/avatar";
import { normalizeXHandle } from "@/lib/x-handle";

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
