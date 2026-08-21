import { describe, expect, test } from "bun:test";

import { shuffle, splitTweetText } from "@/lib/tweet-text";

describe("splitTweetText", () => {
  test("highlights @handles", () => {
    expect(splitTweetText("@tedbrine 🐐")).toEqual([
      { type: "mention", value: "@tedbrine" },
      { type: "text", value: " 🐐" },
    ]);
    expect(splitTweetText("thanks @lassejv and @MaxKtz!")).toEqual([
      { type: "text", value: "thanks " },
      { type: "mention", value: "@lassejv" },
      { type: "text", value: " and " },
      { type: "mention", value: "@MaxKtz" },
      { type: "text", value: "!" },
    ]);
  });

  test("does not treat emails as mentions", () => {
    expect(splitTweetText("write ted@example.com please")).toEqual([
      { type: "text", value: "write ted@example.com please" },
    ]);
  });
});

describe("shuffle", () => {
  test("keeps the same items", () => {
    expect(shuffle([1, 2, 3]).toSorted()).toEqual([1, 2, 3]);
    expect(shuffle([])).toEqual([]);
    expect(shuffle(["only"])).toEqual(["only"]);
  });
});
