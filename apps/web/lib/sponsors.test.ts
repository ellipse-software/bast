import { describe, expect, test } from "bun:test";

import { parseSponsorAmountUsd, usdToCents } from "@/lib/sponsors";

describe("parseSponsorAmountUsd", () => {
  test("accepts dollar amounts in range", () => {
    expect(parseSponsorAmountUsd(1)).toBe(100);
    expect(parseSponsorAmountUsd(100)).toBe(10_000);
    expect(parseSponsorAmountUsd("25")).toBe(2500);
    expect(parseSponsorAmountUsd(12.5)).toBe(1250);
  });

  test("rejects out of range and invalid values", () => {
    expect(parseSponsorAmountUsd(0)).toBeNull();
    expect(parseSponsorAmountUsd(10_001)).toBeNull();
    expect(parseSponsorAmountUsd("nope")).toBeNull();
    expect(parseSponsorAmountUsd(Number.NaN)).toBeNull();
  });
});

describe("usdToCents", () => {
  test("rounds to nearest cent", () => {
    expect(usdToCents(1)).toBe(100);
    expect(usdToCents(19.99)).toBe(1999);
  });
});
