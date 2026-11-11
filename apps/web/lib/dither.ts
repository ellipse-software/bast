export type RGB = readonly [number, number, number];

/**
 * Night inks, dark to light within each hue.
 * The screen mixes only inside a ramp, so the sky stays navy, the wisps
 * stay a step lighter, and the blossom stays purple.
 */
const skyRamp = [
  [2, 8, 22],
  [6, 16, 40],
  [14, 30, 64],
  [26, 46, 88],
] as const satisfies readonly RGB[];

const cloudRamp = [
  [22, 36, 74],
  [42, 62, 108],
  [74, 96, 146],
  [118, 138, 182],
] as const satisfies readonly RGB[];

const greenRamp = [
  [4, 12, 8],
  [14, 36, 18],
  [32, 70, 34],
  [68, 116, 58],
] as const satisfies readonly RGB[];

const blossomRamp = [
  [32, 18, 52],
  [68, 40, 98],
  [112, 74, 150],
  [158, 114, 188],
] as const satisfies readonly RGB[];

const starRamp = [[176, 190, 214]] as const satisfies readonly RGB[];

export const beachRamps: readonly (readonly RGB[])[] = [
  skyRamp,
  cloudRamp,
  greenRamp,
  blossomRamp,
  starRamp,
];

/** 8×8 Bayer matrix, same order as an ordered-dither screen. Values are 0–63. */
const BAYER_8 = [
  0, 48, 12, 60, 3, 51, 15, 63, 32, 16, 44, 28, 35, 19, 47, 31, 8, 56, 4, 52,
  11, 59, 7, 55, 40, 24, 36, 20, 43, 27, 39, 23, 2, 50, 14, 62, 1, 49, 13, 61,
  34, 18, 46, 30, 33, 17, 45, 29, 10, 58, 6, 54, 9, 57, 5, 53, 42, 26, 38, 22,
  41, 25, 37, 21,
] as const;

function clamp01(value: number): number {
  if (value < 0) return 0;
  if (value > 1) return 1;
  return value;
}

function smoothstep(edge0: number, edge1: number, value: number): number {
  const t = clamp01((value - edge0) / (edge1 - edge0));
  return t * t * (3 - 2 * t);
}

function luma(red: number, green: number, blue: number): number {
  return (red * 0.299 + green * 0.587 + blue * 0.114) / 255;
}

function hueSat(
  red: number,
  green: number,
  blue: number,
): { hue: number; sat: number } {
  const r = red / 255;
  const g = green / 255;
  const b = blue / 255;
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const delta = max - min;
  const light = (max + min) / 2;
  if (delta === 0) return { hue: 0, sat: 0 };

  const sat = delta / (1 - Math.abs(2 * light - 1) || 1);
  let hue = 0;
  if (max === r) hue = ((g - b) / delta) % 6;
  else if (max === g) hue = (b - r) / delta + 2;
  else hue = (r - g) / delta + 4;
  hue *= 60;
  if (hue < 0) hue += 360;
  return { hue, sat };
}

function mix(from: RGB, to: RGB, amount: number): RGB {
  const t = clamp01(amount);
  return [
    Math.round(from[0] + (to[0] - from[0]) * t),
    Math.round(from[1] + (to[1] - from[1]) * t),
    Math.round(from[2] + (to[2] - from[2]) * t),
  ];
}

/**
 * Ordered Bayer screen along one ramp.
 * The matrix biases the sample, then the value rounds to an ink, which is
 * the same decision paged uses for its background.
 */
export function bayerRamp(
  red: number,
  green: number,
  blue: number,
  cellX: number,
  cellY: number,
  ramp: readonly RGB[],
): RGB {
  const first = ramp[0] ?? ([0, 0, 0] as const);
  const last = ramp[ramp.length - 1] ?? first;
  const low = luma(first[0], first[1], first[2]);
  const high = luma(last[0], last[1], last[2]);
  const span = Math.max(high - low, 0.001);
  const f = clamp01((luma(red, green, blue) - low) / span);
  const matrix = BAYER_8[((cellY & 7) * 8 + (cellX & 7)) % BAYER_8.length] ?? 0;
  const steps = Math.max(ramp.length - 1, 1);
  let value = f + (matrix / 64 - 0.25) / steps;
  value -= 0.2 * (1 - smoothstep(0.45, 0.8, value));
  value = clamp01(value);
  const index = Math.min(
    ramp.length - 1,
    Math.max(0, Math.round(value * steps)),
  );
  return ramp[index] ?? first;
}

/**
 * Ordered dither without the hero's dark bias, so a low coverage stays a
 * sparse field instead of snapping to the first ink.
 */
export function bayerCover(
  coverage: number,
  cellX: number,
  cellY: number,
  ramp: readonly RGB[],
): RGB {
  const first = ramp[0] ?? ([0, 0, 0] as const);
  const steps = Math.max(ramp.length - 1, 1);
  const matrix = BAYER_8[((cellY & 7) * 8 + (cellX & 7)) % BAYER_8.length] ?? 0;
  const value = clamp01(coverage + (matrix / 64 - 0.5) / steps);
  const index = Math.min(steps, Math.max(0, Math.round(value * steps)));
  return ramp[index] ?? first;
}

function isBlossom(red: number, green: number, blue: number): boolean {
  const sample = hueSat(red, green, blue);
  return (
    sample.hue > 246 &&
    sample.hue < 335 &&
    sample.sat > 0.12 &&
    red > 16 &&
    red + 4 > green
  );
}

function isFoliage(red: number, green: number, blue: number): boolean {
  return green > red + 6 && green + 10 >= blue && green > 12;
}

function isWisp(red: number, green: number, blue: number): boolean {
  return blue > red + 12 && blue + 6 >= green && luma(red, green, blue) > 0.11;
}

/** A sparse field. Photographic stars disappear at this cell size. */
function isStar(cellX: number, cellY: number): boolean {
  const hash = Math.imul(cellX, 374761393) ^ Math.imul(cellY, 668265263);
  return (hash >>> 0) % 340 === 0;
}

function shapeSky(red: number, green: number, blue: number): RGB {
  const amount = clamp01((luma(red, green, blue) - 0.02) / 0.1);
  return mix([4, 12, 30], [18, 36, 72], amount);
}

function shapeWisp(red: number, green: number, blue: number): RGB {
  const amount = clamp01((luma(red, green, blue) - 0.08) / 0.2);
  return mix([36, 54, 102], [110, 132, 176], amount);
}

function shapeFoliage(red: number, green: number, blue: number): RGB {
  const amount = clamp01((luma(red, green, blue) - 0.02) / 0.14);
  return mix([6, 18, 10], [40, 78, 36], amount);
}

function shapeBlossom(red: number, green: number, blue: number): RGB {
  const amount = clamp01((luma(red, green, blue) - 0.04) / 0.16);
  return mix([40, 22, 64], [140, 96, 176], 0.2 + amount * 0.75);
}

export function beachInk(
  red: number,
  green: number,
  blue: number,
  cellX: number,
  cellY: number,
  phase = 0,
): RGB {
  let sample: RGB = [red, green, blue];
  let ramp: readonly RGB[] = skyRamp;
  if (isBlossom(red, green, blue)) {
    sample = shapeBlossom(red, green, blue);
    ramp = blossomRamp;
  } else if (isFoliage(red, green, blue)) {
    sample = shapeFoliage(red, green, blue);
    ramp = greenRamp;
  } else if (isWisp(red, green, blue)) {
    sample = shapeWisp(red, green, blue);
    ramp = cloudRamp;
  } else if (isStar(cellX, cellY)) {
    return starRamp[0];
  } else {
    sample = shapeSky(red, green, blue);
  }

  return bayerRamp(
    sample[0],
    sample[1],
    sample[2],
    cellX + phase,
    cellY + (phase >> 1),
    ramp,
  );
}
