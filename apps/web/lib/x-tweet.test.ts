import { describe, expect, test } from "bun:test";

import { parseFxStatus, parseTweetUrl } from "@/lib/x-tweet";

describe("parseTweetUrl", () => {
  test("accepts x.com and twitter.com status URLs", () => {
    expect(
      parseTweetUrl("https://x.com/maxktz/status/2082138847714877727"),
    ).toEqual({ id: "2082138847714877727", handle: "maxktz" });
    expect(
      parseTweetUrl(
        "https://twitter.com/LasseJV/status/2082077775716815065?s=20",
      ),
    ).toEqual({ id: "2082077775716815065", handle: "lassejv" });
    expect(
      parseTweetUrl("https://www.x.com/i/web/status/2082138847714877727"),
    ).toEqual({ id: "2082138847714877727", handle: null });
  });

  test("rejects non-tweet URLs", () => {
    expect(parseTweetUrl("https://x.com/maxktz")).toBeNull();
    expect(parseTweetUrl("https://example.com/status/1")).toBeNull();
    expect(parseTweetUrl("not a url")).toBeNull();
  });
});

describe("parseFxStatus", () => {
  test("reads v2 status payload", () => {
    expect(
      parseFxStatus({
        code: 200,
        status: {
          id: "2082138847714877727",
          url: "https://x.com/maxktz/status/2082138847714877727",
          text: "@tedbrine 🐐",
          author: {
            screen_name: "maxktz",
            name: "Max Katz",
            avatar_url:
              "https://pbs.twimg.com/profile_images/1/azNjKOSH_normal.jpg",
            verification: { verified: true },
          },
        },
      }),
    ).toEqual({
      id: "2082138847714877727",
      url: "https://x.com/maxktz/status/2082138847714877727",
      text: "@tedbrine 🐐",
      username: "maxktz",
      profile: {
        name: "Max Katz",
        avatarUrl: "https://pbs.twimg.com/profile_images/1/azNjKOSH_400x400.jpg",
        verified: true,
      },
    });
  });

  test("reads v1 tweet payload", () => {
    const parsed = parseFxStatus({
      tweet: {
        id: "1",
        text: "hello",
        author: { screen_name: "jack", name: "jack" },
      },
    });
    expect(parsed?.username).toBe("jack");
    expect(
      parseFxStatus({
        status: {
          id: "1",
          text: "hi",
          author: { screen_name: "Itri_SpecialGuy", name: "Itri" },
        },
      })?.username,
    ).toBe("Itri_SpecialGuy");
    expect(parsed?.text).toBe("hello");
    expect(parsed?.url).toBe("https://x.com/jack/status/1");
  });

  test("returns null for malformed payloads", () => {
    expect(parseFxStatus(null)).toBeNull();
    expect(parseFxStatus({})).toBeNull();
    expect(parseFxStatus({ status: { id: "1", text: "hi" } })).toBeNull();
  });
});
