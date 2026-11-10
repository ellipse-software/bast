"use client";

import { type CSSProperties, useLayoutEffect, useRef } from "react";

import { bayerRamp, type RGB } from "@/lib/dither";

type ChamberPalette = {
	wash: string;
	outline: string;
	fill: string;
	ruleClass: string;
	cipher: string;
	spark: string;
	trail: readonly [
		readonly [string, number],
		readonly [string, number],
		readonly [string, number],
		readonly [string, number],
	];
};

const defaultPalette: ChamberPalette = {
	wash: "linear-gradient(to top, rgb(76 29 149 / 0.4) 0%, rgb(76 29 149 / 0.14) 34%, rgb(76 29 149 / 0.04) 58%, transparent 82%)",
	outline: "#5b21b6",
	fill: "#0a0a0a",
	ruleClass: "border-[#5b21b6]",
	cipher: "196, 181, 253",
	spark: "124, 58, 237",
	trail: [
		["216, 180, 254", 0.72],
		["167, 139, 250", 0.55],
		["139, 92, 246", 0.28],
		["91, 33, 182", 0],
	],
};

const nightPalette: ChamberPalette = {
	wash: "#05080f",
	outline: "#7c3aed",
	fill: "#05080f",
	ruleClass: "border-[#7c3aed]",
	cipher: "216, 180, 254",
	spark: "167, 139, 250",
	trail: [
		["216, 180, 254", 0.72],
		["196, 181, 253", 0.55],
		["167, 139, 250", 0.28],
		["124, 58, 237", 0],
	],
};

const NIGHT_WASH_RAMP = [
	[5, 8, 15],
	[36, 18, 64],
	[64, 34, 116],
	[98, 58, 166],
	[136, 96, 208],
] as const satisfies readonly RGB[];

const WASH_PLATE: RGB = [5, 8, 15];
const WASH_LIFT: RGB = [139, 92, 246];

function washAlpha(fromBottom: number): number {
	const stops = [
		[0, 0.55],
		[0.55, 0.32],
		[0.78, 0.14],
		[0.96, 0],
	] as const;
	if (fromBottom >= 0.96) return 0;
	for (let index = 0; index < stops.length - 1; index += 1) {
		const [start, startAlpha] = stops[index] ?? [0, 0];
		const [end, endAlpha] = stops[index + 1] ?? [1, 0];
		if (fromBottom < start || fromBottom > end) continue;
		const span = end - start;
		const t = span === 0 ? 0 : (fromBottom - start) / span;
		return startAlpha + (endAlpha - startAlpha) * t;
	}
	return 0;
}

function washSample(fromBottom: number): RGB {
	const alpha = washAlpha(fromBottom);
	return [
		Math.round(WASH_PLATE[0] + (WASH_LIFT[0] - WASH_PLATE[0]) * alpha),
		Math.round(WASH_PLATE[1] + (WASH_LIFT[1] - WASH_PLATE[1]) * alpha),
		Math.round(WASH_PLATE[2] + (WASH_LIFT[2] - WASH_PLATE[2]) * alpha),
	];
}

/** Bayer screen of a lighter violet wash. Same 3px cell as the night hero. */
function DitherWash() {
	const canvasRef = useRef<HTMLCanvasElement>(null);

	useLayoutEffect(() => {
		const canvas = canvasRef.current;
		const parent = canvas?.parentElement ?? null;
		if (!canvas || !parent) return;
		const context = canvas.getContext("2d", { alpha: false });
		if (!context) return;

		const ink = document.createElement("canvas");
		const inkContext = ink.getContext("2d", { alpha: false });
		if (!inkContext) return;

		let frame = 0;

		function paint() {
			const width = parent?.clientWidth ?? 0;
			const height = parent?.clientHeight ?? 0;
			if (!context || !inkContext || width < 2 || height < 2) return;

			const cssCell = 3;
			const cols = Math.max(1, Math.ceil(width / cssCell));
			const rows = Math.max(1, Math.ceil(height / cssCell));
			const pixelRatio = Math.min(window.devicePixelRatio || 1, 2);
			const cell = Math.max(1, Math.round(cssCell * pixelRatio));
			if (ink.width !== cols || ink.height !== rows) {
				ink.width = cols;
				ink.height = rows;
			}

			const image = inkContext.createImageData(cols, rows);
			const pixels = image.data;
			for (let y = 0; y < rows; y += 1) {
				const fromBottom = 1 - y / Math.max(rows - 1, 1);
				const sample = washSample(fromBottom);
				for (let x = 0; x < cols; x += 1) {
					const [red, green, blue] = bayerRamp(
						sample[0],
						sample[1],
						sample[2],
						x,
						y,
						NIGHT_WASH_RAMP,
					);
					const index = (y * cols + x) * 4;
					pixels[index] = red;
					pixels[index + 1] = green;
					pixels[index + 2] = blue;
					pixels[index + 3] = 255;
				}
			}

			inkContext.putImageData(image, 0, 0);
			const outWidth = cols * cell;
			const outHeight = rows * cell;
			if (canvas && (canvas.width !== outWidth || canvas.height !== outHeight)) {
				canvas.width = outWidth;
				canvas.height = outHeight;
			}
			context.imageSmoothingEnabled = false;
			context.drawImage(ink, 0, 0, outWidth, outHeight);
		}

		paint();
		const observer = new ResizeObserver(() => {
			cancelAnimationFrame(frame);
			frame = requestAnimationFrame(paint);
		});
		observer.observe(parent);
		return () => {
			cancelAnimationFrame(frame);
			observer.disconnect();
		};
	}, []);

	return (
		<canvas
			ref={canvasRef}
			aria-hidden
			className="pointer-events-none absolute inset-0 h-full w-full [image-rendering:pixelated]"
		/>
	);
}

const wordmarkTextClass =
	"whitespace-nowrap font-medium leading-none tracking-[-0.045em]";

/** Cap at Tailwind `text-8xl` (6rem) so zoomed-out / ultra-wide doesn't inflate forever. */
const WORDMARK_MAX_REM = 28;

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

const HEX = "0123456789abcdef";

type CipherGlyph = {
	ch: string;
	x: number;
	y: number;
	alpha: number;
	nextFlip: number;
	/** 0 idle → 1 peak; fades back after a purple flash. */
	spark: number;
	nextSpark: number;
};

/** Sparse hex field — barely readable cipher dust in the wash. */
function buildCipherField(width: number, height: number): CipherGlyph[] {
	const cell = 18;
	const glyphs: CipherGlyph[] = [];
	const cols = Math.ceil(width / cell);
	// Snap the last row to the footer floor so cipher runs to the bottom edge.
	const top = cell * 0.72;
	const bottom = Math.max(top, height - 1);
	const rows = Math.max(1, Math.round((bottom - top) / cell) + 1);

	for (let row = 0; row < rows; row += 1) {
		for (let col = 0; col < cols; col += 1) {
			// Leave most cells empty so the field reads as dust, not a grid.
			if (Math.random() > 0.34) continue;
			const t = rows === 1 ? 1 : row / (rows - 1);
			const y = top + t * (bottom - top);
			const depth = Math.min(1, Math.max(0, y / height));
			// Stronger near the floor; dissolve toward the top of the wash.
			const alpha = 0.055 + depth * depth * 0.12;
			glyphs.push({
				ch: HEX[(Math.random() * 16) | 0]!,
				x: col * cell + cell * 0.18,
				y,
				alpha,
				nextFlip: rand(500, 3400),
				spark: 0,
				nextSpark: rand(2000, 14000),
			});
		}
	}
	return glyphs;
}

/** Cloud-chamber ionization streaks rising through the wash. */
function ChamberField({ palette }: { palette: ChamberPalette }) {
	const canvasRef = useRef<HTMLCanvasElement>(null);

	useLayoutEffect(() => {
		const canvasNode = canvasRef.current;
		const parentNode = canvasNode?.parentElement ?? null;
		if (!canvasNode || !parentNode) return;

		const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
		const prefersReduced = reduced.matches;

		const ctxNode = canvasNode.getContext("2d", { alpha: true });
		if (!ctxNode) return;

		// Typed aliases so nested closures see non-null DOM handles.
		const canvas: HTMLCanvasElement = canvasNode;
		const parent: HTMLElement = parentNode;
		const ctx: CanvasRenderingContext2D = ctxNode;

		let tracks: ChamberTrack[] = [];
		let glyphs: CipherGlyph[] = [];
		let raf = 0;
		let last = performance.now();
		let spawnAcc = 0;
		let nextSpawnIn = rand(200, 480);
		let running = true;
		let w = 0;
		let h = 0;
		let dpr = 1;

		function paintCipher(animate: boolean, dt: number) {
			if (glyphs.length === 0) return;
			ctx.globalCompositeOperation = "source-over";
			ctx.font =
				"13px ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";
			ctx.textBaseline = "alphabetic";
			ctx.textAlign = "left";

			for (const g of glyphs) {
				if (animate) {
					g.nextFlip -= dt;
					if (g.nextFlip <= 0) {
						g.ch = HEX[(Math.random() * 16) | 0]!;
						g.nextFlip = rand(700, 3600);
					}

					if (g.spark > 0) {
						g.spark = Math.max(0, g.spark - dt / 900);
					} else {
						g.nextSpark -= dt;
						if (g.nextSpark <= 0) {
							// Sparse: most ticks no-op, occasionally one letter flashes purple.
							if (Math.random() < 0.1) g.spark = 1;
							g.nextSpark = rand(3200, 12000);
						}
					}
				}

				if (g.spark > 0) {
					const s = g.spark * g.spark;
					ctx.fillStyle = `rgba(${palette.spark}, ${Math.min(0.95, g.alpha + 0.55 * s)})`;
				} else {
					ctx.fillStyle = `rgba(${palette.cipher}, ${g.alpha})`;
				}
				ctx.fillText(g.ch, g.x, g.y);
			}
		}

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
			glyphs = buildCipherField(w, h);
			if (prefersReduced) {
				ctx.clearRect(0, 0, w, h);
				paintCipher(false, 0);
			}
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
			const [tip, mid, fade, end] = palette.trail;
			grad.addColorStop(0, `rgba(${tip[0]}, ${tip[1] * alpha})`);
			grad.addColorStop(0.5, `rgba(${mid[0]}, ${mid[1] * alpha})`);
			grad.addColorStop(0.78, `rgba(${fade[0]}, ${fade[1] * alpha})`);
			grad.addColorStop(1, `rgba(${end[0]}, ${end[1]})`);

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
			paintCipher(true, dt);

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

		const ro = new ResizeObserver(() => {
			resize();
		});
		ro.observe(parent);

		if (prefersReduced) {
			return () => {
				ro.disconnect();
			};
		}

		for (let i = 0; i < 7; i += 1) {
			const t = spawnTrack(w, h, tracks);
			if (!t) break;
			t.drawn = rand(0.15, 0.7);
			tracks.push(t);
		}

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
	}, [palette]);

	return (
		<canvas
			ref={canvasRef}
			aria-hidden
			className="pointer-events-none absolute inset-0 [mask-image:linear-gradient(to_top,black_0%,black_62%,rgba(0,0,0,0.5)_86%,transparent_100%)] [-webkit-mask-image:linear-gradient(to_top,black_0%,black_62%,rgba(0,0,0,0.5)_86%,transparent_100%)]"
		/>
	);
}

/**
 * Stroke is centered on the glyph edge; the fill layer covers the inner half.
 * Visible rim ≈ half the stroke width. Use 2× so the rim matches the 1px rule.
 */
function glyphStyles(palette: ChamberPalette): {
	outline: CSSProperties;
	fill: CSSProperties;
} {
	return {
		outline: {
			color: "transparent",
			WebkitTextStrokeColor: palette.outline,
			WebkitTextStrokeWidth: "0.012em",
		},
		fill: { color: palette.fill },
	};
}

/** Full-bleed `bast.sh` mark. */
export function FooterWordmark({
	tone = "default",
}: {
	tone?: "default" | "night";
} = {}) {
	const night = tone === "night";
	const palette = night ? nightPalette : defaultPalette;
	const { outline, fill } = glyphStyles(palette);
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

			const rootPx = Number.parseFloat(
				getComputedStyle(document.documentElement).fontSize,
			);
			const maxPx =
				(Number.isFinite(rootPx) && rootPx > 0 ? rootPx : 16) *
				WORDMARK_MAX_REM;

			let low = 8;
			let high = Math.min(Math.max(available * 0.55, 9), maxPx);

			for (let i = 0; i < 24; i += 1) {
				const mid = (low + high) / 2;
				mask.style.fontSize = `${mid}px`;
				if (probe.scrollWidth > available) {
					high = mid;
				} else {
					low = mid;
				}
			}

			let size = low;
			mask.style.fontSize = `${size}px`;
			const measured = probe.scrollWidth;
			if (measured > 0) {
				size = low * (available / measured);
			}
			mask.style.fontSize = `${Math.min(size, maxPx)}px`;
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
			{night ? (
				<DitherWash />
			) : (
				<div
					aria-hidden
					className="absolute inset-x-0 bottom-0 top-0"
					style={{ backgroundImage: palette.wash }}
				/>
			)}
			<ChamberField palette={palette} />
			<div
				ref={maskRef}
				className={`relative flex h-[0.78em] w-full justify-center overflow-hidden border-b border-b-[1px] sm:border-b-0 ${palette.ruleClass}`}
			>
				<div className="relative w-max max-w-none max-sm:-translate-x-[3px]">
					<p
						data-wordmark-probe
						className={`pointer-events-none absolute left-0 top-0 ${wordmarkTextClass}`}
						style={outline}
					>
						bast.sh
					</p>
					<p className={`relative ${wordmarkTextClass}`} style={fill}>
						bast.sh
					</p>
				</div>
			</div>
		</div>
	);
}
