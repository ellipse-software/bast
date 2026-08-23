import { describe, expect, test } from "bun:test";

import { parseSponsorAmountUsd, parseSponsorInterval, usdToCents } from "@/lib/sponsors";

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

describe("parseSponsorInterval", () => {
  test("defaults missing values to one time", () => {
    expect(parseSponsorInterval(undefined)).toBe("once");
    expect(parseSponsorInterval(null)).toBe("once");
    expect(parseSponsorInterval("")).toBe("once");
  });

  test("accepts once and month", () => {
    expect(parseSponsorInterval("once")).toBe("once");
    expect(parseSponsorInterval("month")).toBe("month");
  });

  test("rejects unknown values", () => {
    expect(parseSponsorInterval("year")).toBeNull();
    expect(parseSponsorInterval(1)).toBeNull();
  });
});

describe("usdToCents", () => {
  test("rounds to nearest cent", () => {
    expect(usdToCents(1)).toBe(100);
    expect(usdToCents(19.99)).toBe(1999);
  });
});
