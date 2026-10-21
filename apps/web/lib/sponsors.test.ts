import { describe, expect, test } from "bun:test";

import {
  parseSponsorAmountUsd,
  parseSponsorInterval,
  resolveSponsors,
  sponsors,
  usdToCents,
} from "@/lib/sponsors";

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

describe("resolveSponsors", () => {
  test("keeps named site sponsors and sorts by amount", () => {
    const listed = resolveSponsors([
      {
        href: "https://x.com/leodev",
        amount: 20,
      },
      {
        href: "https://rdke.dev",
        name: "Tom Benjamin Radtke",
        avatar: "http://galileo.ted.ac/dQbmFFHNaO.jpg",
        message: "Finally a clean alternative for people with hundreds of hosts.",
        amount: 250,
      },
    ]);

    expect(listed).toEqual([
      {
        href: "https://rdke.dev",
        name: "Tom Benjamin Radtke",
        username: null,
        avatar: "https://galileo.ted.ac/dQbmFFHNaO.jpg",
        message: "Finally a clean alternative for people with hundreds of hosts.",
        amount: 250,
        anonymous: undefined,
      },
      {
        href: "https://x.com/leodev",
        name: "leodev",
        username: "leodev",
        avatar: undefined,
        message: undefined,
        amount: 20,
        anonymous: undefined,
      },
    ]);
  });

  test("drops entries that are neither an X profile nor a named sponsor", () => {
    expect(
      resolveSponsors([{ href: "https://example.com", amount: 10 }]),
    ).toEqual([]);
  });

  test("lists Tom Benjamin Radtke first on the landing page", () => {
    const listed = resolveSponsors(sponsors);
    expect(listed[0]).toMatchObject({
      href: "https://rdke.dev",
      name: "Tom Benjamin Radtke",
      amount: 250,
    });
  });
});
