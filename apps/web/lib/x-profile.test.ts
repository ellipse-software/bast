import { describe, expect, test } from "bun:test";

import { parseFxProfile, upgradeTwitterAvatarUrl } from "@/lib/x-profile";

describe("upgradeTwitterAvatarUrl", () => {
  test("promotes _normal and _bigger to _400x400", () => {
    expect(
      upgradeTwitterAvatarUrl(
        "https://pbs.twimg.com/profile_images/1/azNjKOSH_normal.jpg",
      ),
    ).toBe("https://pbs.twimg.com/profile_images/1/azNjKOSH_400x400.jpg");
    expect(
      upgradeTwitterAvatarUrl(
        "https://pbs.twimg.com/profile_images/1/azNjKOSH_bigger.png",
      ),
    ).toBe("https://pbs.twimg.com/profile_images/1/azNjKOSH_400x400.png");
  });

  test("leaves original-size URLs alone", () => {
    const original = "https://pbs.twimg.com/profile_images/1/azNjKOSH.jpg";
    expect(upgradeTwitterAvatarUrl(original)).toBe(original);
  });
});

describe("parseFxProfile", () => {
  test("reads name, avatar, and verified", () => {
    expect(
      parseFxProfile({
        code: 200,
        user: {
          name: " jack ",
          avatar_url:
            "https://pbs.twimg.com/profile_images/1/azNjKOSH_normal.jpg",
          verification: { verified: true, type: "individual" },
        },
      }),
    ).toEqual({
      name: "jack",
      avatarUrl: "https://pbs.twimg.com/profile_images/1/azNjKOSH_400x400.jpg",
      verified: true,
    });
  });

  test("returns null for missing or malformed payloads", () => {
    expect(parseFxProfile(null)).toBeNull();
    expect(parseFxProfile({})).toBeNull();
    expect(parseFxProfile({ user: { avatar_url: "https://x.com/a" } })).toBeNull();
    expect(
      parseFxProfile({
        user: { name: "Ada", verification: { verified: false } },
      }),
    ).toEqual({
      name: "Ada",
      avatarUrl: null,
      verified: false,
    });
  });
});
