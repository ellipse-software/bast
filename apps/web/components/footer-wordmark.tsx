"use client";

import { useLayoutEffect, useRef, type CSSProperties } from "react";

const washStyle: CSSProperties = {
  backgroundImage:
    "linear-gradient(to top, rgb(76 29 149 / 0.4) 0%, rgb(76 29 149 / 0.14) 34%, rgb(76 29 149 / 0.04) 58%, transparent 82%)",
};

/** Diagonal terminal dots on page ink — clipped into the glyph fill only. */
function dotFill(tile: number): string {
  const a = Math.round(tile * 0.17);
  const b = Math.round(tile * 0.67);
  return `url("data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="${tile}" height="${tile}" shape-rendering="crispEdges">
      <rect width="${tile}" height="${tile}" fill="#0a0a0a"/>
      <rect x="${a}" y="${a}" width="1" height="1" fill="#7c3aed" fill-opacity="0.55"/>
      <rect x="${b}" y="${b}" width="1" height="1" fill="#7c3aed" fill-opacity="0.55"/>
    </svg>`,
  )}")`;
}

const DOT_FILL = dotFill(12);

const OUTLINE = "#5b21b6";

/**
 * Stroke is centered on the glyph edge; the fill layer covers the inner half.
 * Visible rim ≈ half the stroke width. Use 2× so the rim matches the 1px rule.
 */
const outlineStyle: CSSProperties = {
  color: "transparent",
  WebkitTextStrokeColor: OUTLINE,
  WebkitTextStrokeWidth: "0.012em",
};

const fillStyle: CSSProperties = {
  backgroundColor: "#0a0a0a",
  backgroundImage: DOT_FILL,
  backgroundRepeat: "repeat",
  backgroundSize: "14px 14px",
  WebkitBackgroundClip: "text",
  backgroundClip: "text",
  WebkitTextFillColor: "transparent",
  color: "transparent",
};

const fillStyleMobile: CSSProperties = {
  ...fillStyle,
  // Same tile scaled down: dots shrink and spacing tightens together.
  backgroundSize: "9px 9px",
};

const wordmarkTextClass =
  "whitespace-nowrap font-medium leading-none tracking-[-0.045em]";

type ChamberTrack = {
  x: number;
  /** Always the separator (canvas bottom). */
  y: number;
  length: number;
  /** 0→1 drawn extent of the trail. */
  drawn: number;
  /** How fast the trail grows (fraction of length per ms). */
  grow: number;
  /** Remaining life after fully drawn; counts down in ms. */
  fadeMs: number;
  lifeMs: number;
  width: number;
};

function rand(min: number, max: number) {
  return min + Math.random() * (max - min);
}

/** Prefer an x that sits away from live tracks — stops clumping on the rule. */
function pickSpawnX(width: number, existing: ChamberTrack[]): number | null {
  const margin = width * 0.04;
  const minGap = Math.max(18, width * 0.055);
  let bestX = rand(margin, width - margin);
  let bestGap = -1;

  for (let i = 0; i < 16; i += 1) {
    const x = rand(margin, width - margin);
    let nearest = Infinity;
    for (const t of existing) {
      nearest = Math.min(nearest, Math.abs(t.x - x));
    }
    if (existing.length === 0) return x;
    if (nearest > bestGap) {
      bestGap = nearest;
      bestX = x;
    }
  }

  if (bestGap < minGap * 0.55) return null;
  return bestX;
}

/**
 * Birth on the separator. Clears the wordmark, climbs into the wash, and
 * stops short of the top so trails dissolve instead of hitting the clip edge.
 */
function spawnTrack(
  width: number,
  height: number,
  existing: ChamberTrack[],
): ChamberTrack | null {
  const x = pickSpawnX(width, existing);
  if (x === null) return null;
  const minLen = height * 0.7;
  const maxLen = height * 0.94;
  return {
    x,
    y: height,
    length: rand(minLen, maxLen),
    drawn: 0,
    grow: rand(0.0011, 0.0024),
    fadeMs: rand(2200, 4200),
    lifeMs: rand(2200, 4200),
    width: rand(1.0, 1.75),
  };
}

/** Cloud-chamber ionization streaks rising through the purple wash. */
function ChamberField() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useLayoutEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const parent = canvas.parentElement;
    if (!parent) return;

    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
    if (reduced.matches) return;

    const ctx = canvas.getContext("2d", { alpha: true });
    if (!ctx) return;

    let tracks: ChamberTrack[] = [];
    let raf = 0;
    let last = performance.now();
    let spawnAcc = 0;
    let nextSpawnIn = rand(200, 480);
    let running = true;
    let w = 0;
    let h = 0;
    let dpr = 1;

    function resize() {
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      w = parent.clientWidth;
      h = parent.clientHeight;
      if (w <= 0 || h <= 0) return;
      canvas.width = Math.floor(w * dpr);
      canvas.height = Math.floor(h * dpr);
      canvas.style.width = `${w}px`;
      canvas.style.height = `${h}px`;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    function strokeTrail(
      x0: number,
      y0: number,
      len: number,
      drawn: number,
      width: number,
      alpha: number,
    ) {
      if (drawn <= 0.001 || alpha <= 0.01) return;
      // Straight up from the separator.
      const x1 = x0;
      const y1 = y0 - len * drawn;

      const grad = ctx.createLinearGradient(x0, y0, x1, y1);
      // Soft tip dissolve — never a hard stop against the chamber ceiling.
      grad.addColorStop(0, `rgba(216, 180, 254, ${0.72 * alpha})`);
      grad.addColorStop(0.5, `rgba(167, 139, 250, ${0.55 * alpha})`);
      grad.addColorStop(0.78, `rgba(139, 92, 246, ${0.28 * alpha})`);
      grad.addColorStop(1, `rgba(91, 33, 182, 0)`);

      ctx.strokeStyle = grad;
      ctx.lineWidth = width;
      ctx.lineCap = "round";
      ctx.beginPath();
      ctx.moveTo(x0, y0);
      ctx.lineTo(x1, y1);
      ctx.stroke();
    }

    function frame(now: number) {
      if (!running) return;
      const dt = Math.min(48, now - last);
      last = now;

      spawnAcc += dt;
      while (spawnAcc >= nextSpawnIn) {
        spawnAcc -= nextSpawnIn;
        nextSpawnIn = rand(180, 520);
        if (tracks.length < 16) {
          const track = spawnTrack(w, h, tracks);
          if (track) tracks.push(track);
        }
      }

      ctx.clearRect(0, 0, w, h);
      ctx.globalCompositeOperation = "lighter";

      const next: ChamberTrack[] = [];
      for (const t of tracks) {
        if (t.drawn < 1) {
          t.drawn = Math.min(1, t.drawn + t.grow * dt);
        } else {
          t.lifeMs -= dt;
        }
        if (t.lifeMs <= 0) continue;

        const fade =
          t.drawn < 1
            ? 0.85 + t.drawn * 0.15
            : Math.max(0, t.lifeMs / t.fadeMs);

        strokeTrail(t.x, t.y, t.length, t.drawn, t.width, fade);
        next.push(t);
      }
      tracks = next;

      raf = requestAnimationFrame(frame);
    }

    resize();
    for (let i = 0; i < 7; i += 1) {
      const t = spawnTrack(w, h, tracks);
      if (!t) break;
      t.drawn = rand(0.15, 0.7);
      tracks.push(t);
    }

    const ro = new ResizeObserver(() => {
      resize();
    });
    ro.observe(parent);

    const onVisibility = () => {
      if (document.hidden) {
        running = false;
        cancelAnimationFrame(raf);
      } else {
        running = true;
        last = performance.now();
        raf = requestAnimationFrame(frame);
      }
    };
    document.addEventListener("visibilitychange", onVisibility);

    raf = requestAnimationFrame(frame);

    return () => {
      running = false;
      cancelAnimationFrame(raf);
      ro.disconnect();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      aria-hidden
      className="pointer-events-none absolute inset-0 motion-reduce:hidden [mask-image:linear-gradient(to_top,black_0%,black_62%,rgba(0,0,0,0.5)_86%,transparent_100%)] [-webkit-mask-image:linear-gradient(to_top,black_0%,black_62%,rgba(0,0,0,0.5)_86%,transparent_100%)]"
    />
  );
}

/** Full-bleed `bast.sh` mark. */
export function FooterWordmark() {
  const maskRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const mask = maskRef.current;
    if (!mask) return;

    function fit() {
      if (!mask) return;
      const probe = mask.querySelector("[data-wordmark-probe]");
      if (!(probe instanceof HTMLElement)) return;

      const available = mask.clientWidth;
      if (available <= 0) return;

      let low = 8;
      let high = Math.max(available * 0.55, 9);

      for (let i = 0; i < 24; i += 1) {
        const mid = (low + high) / 2;
        mask.style.fontSize = `${mid}px`;
        if (probe.scrollWidth > available) {
          high = mid;
        } else {
          low = mid;
        }
      }

      mask.style.fontSize = `${low}px`;
      const measured = probe.scrollWidth;
      if (measured > 0) {
        mask.style.fontSize = `${low * (available / measured)}px`;
      }
    }

    fit();
    void document.fonts.ready.then(fit);

    const observer = new ResizeObserver(fit);
    observer.observe(mask);
    window.addEventListener("resize", fit);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", fit);
    };
  }, []);

  return (
    <div
      aria-hidden
      className="pointer-events-none relative mt-4 w-full overflow-hidden select-none pt-28 max-sm:pt-36"
    >
      <div
        aria-hidden
        className="absolute inset-x-0 bottom-0 top-0"
        style={washStyle}
      />
      <ChamberField />
      <div
        ref={maskRef}
        className="relative flex h-[0.78em] w-full justify-center overflow-hidden border-b border-b-[1px] border-[#5b21b6] sm:border-b-[0.5px]"
      >
        <div className="relative w-max max-w-none max-sm:-translate-x-[3px]">
          <p
            data-wordmark-probe
            className={`pointer-events-none absolute left-0 top-0 ${wordmarkTextClass}`}
            style={outlineStyle}
          >
            bast.sh
          </p>
          <p
            className={`relative hidden sm:block ${wordmarkTextClass}`}
            style={fillStyle}
          >
            bast.sh
          </p>
          <p
            className={`relative sm:hidden ${wordmarkTextClass}`}
            style={fillStyleMobile}
          >
            bast.sh
          </p>
        </div>
      </div>
    </div>
  );
}
