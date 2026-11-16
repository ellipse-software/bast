import type { CSSProperties } from "react";

export const nightTheme = {
  "--background": "#020814",
  "--foreground": "#e8eef6",
  "--muted": "#9aa8bc",
  "--border": "rgba(255, 255, 255, 0.15)",
  "--surface": "rgba(255, 255, 255, 0.05)",
  "--accent": "#c6b6f2",
} as CSSProperties;

export const nightSurface = {
  "--background": "#05080f",
  "--foreground": "#e8eef6",
  "--muted": "#9aa8bc",
  "--border": "rgba(255, 255, 255, 0.15)",
  "--surface": "#0c0f14",
  "--accent": "#c6b6f2",
} as CSSProperties;

export const nightMenuClass =
  "overflow-hidden rounded-xl border border-white/15 bg-[#0c0f14] text-[#e8eef6] shadow-[0_28px_64px_-28px_rgba(0,0,0,0.72)]";
