import { describe, expect, test } from "bun:test";

import { bayerRamp, beachInk, beachRamps, type RGB } from "@/lib/dither";

const blackAndWhite: readonly RGB[] = [
  [0, 0, 0],
  [255, 255, 255],
];

function lights(value: number): number {
  let light = 0;
  for (let y = 0; y < 8; y++) {
    for (let x = 0; x < 8; x++) {
      const ink = bayerRamp(value, value, value, x, y, blackAndWhite);
      if (ink[0] === 255) light += 1;
    }
  }
  return light;
}

describe("bayerRamp", () => {
  test("keeps a black field mostly black and a white field white", () => {
    expect(bayerRamp(0, 0, 0, 0, 0, blackAndWhite)).toEqual([0, 0, 0]);
    expect(lights(0)).toBeLessThan(12);
    expect(bayerRamp(255, 255, 255, 0, 0, blackAndWhite)).toEqual([
      255, 255, 255,
    ]);
    expect(lights(255)).toBe(64);
  });

  test("splits a boundary color across the Bayer matrix", () => {
    expect(bayerRamp(128, 128, 128, 0, 0, blackAndWhite)).toEqual([0, 0, 0]);
    expect(bayerRamp(128, 128, 128, 1, 0, blackAndWhite)).toEqual([
      255, 255, 255,
    ]);
  });

  test("uses more of the light ink as the sample gets lighter", () => {
    const dark = lights(64);
    const mid = lights(128);
    expect(dark).toBeGreaterThan(8);
    expect(dark).toBeLessThan(28);
    expect(mid).toBeGreaterThan(dark);
  });

  test("keeps the night sky dark and the blossom purple", () => {
    const sky = beachInk(8, 16, 40, 1, 1);
    expect(sky[2]).toBeGreaterThan(sky[0]);
    expect(sky[2]).toBeLessThan(100);

    const blossom = new Set<string>();
    for (let y = 0; y < 8; y++) {
      for (let x = 0; x < 8; x++) {
        blossom.add(beachInk(72, 44, 108, x, y).join(","));
      }
    }
    expect(blossom.has("112,74,150") || blossom.has("68,40,98")).toBe(true);
  });

  test("shifts dither grain with phase and keeps stars fixed", () => {
    const still = beachInk(72, 44, 108, 0, 0);
    expect(beachInk(72, 44, 108, 0, 0, 1)).not.toEqual(still);

    for (let y = 0; y < 48; y++) {
      for (let x = 0; x < 48; x++) {
        const ink = beachInk(8, 16, 40, x, y);
        if (ink[0] !== 176 || ink[1] !== 190 || ink[2] !== 214) continue;
        expect(beachInk(8, 16, 40, x, y, 5)).toEqual(ink);
        return;
      }
    }
    throw new Error("expected a star cell");
  });

  test("only returns ramp inks", () => {
    const ramp: readonly RGB[] = [
      [24, 12, 64],
      [246, 214, 78],
    ];

    for (let y = 0; y < 8; y++) {
      for (let x = 0; x < 8; x++) {
        expect(ramp).toContainEqual(bayerRamp(80, 40, 90, x, y, ramp));
        const ink = beachInk(220, 160, 90, x, y);
        const known = beachRamps.flat();
        expect(known).toContainEqual(ink);
      }
    }
  });
});
