import type { CSSProperties } from "react";

import { InstallCommand } from "@/components/install-command";
import { winget } from "@/flags";
import { pageMaxWidthClass } from "@/lib/layout";
import { supportsWindowsRelease } from "@/lib/releases";

type PreFooterProps = {
	version?: string | null;
	tone?: "default" | "night";
};

const nightGradientStyle: CSSProperties = {
	backgroundColor: "#05080f",
	backgroundImage:
		"linear-gradient(to top, #22143c 0%, #141028 42%, #05080f 100%)",
};

const nightDotStyle: CSSProperties = {
	backgroundImage:
		"radial-gradient(circle at 1px 1px, rgb(232 238 246 / 0.28) 1px, transparent 0)",
	backgroundSize: "22px 22px",
	maskImage: "linear-gradient(to top, #000 0%, transparent 100%)",
	WebkitMaskImage: "linear-gradient(to top, #000 0%, transparent 100%)",
};

const nightInstallTheme = {
	"--background": "#070b14",
	"--foreground": "#e7eef8",
	"--muted": "#b4c0d2",
	"--border": "rgba(231, 238, 248, 0.22)",
	"--surface": "#05080f",
	"--accent": "#c6b6f2",
} as CSSProperties;

const patternStyle: CSSProperties = {
	backgroundColor: "color-mix(in srgb, var(--accent) 42%, #07040f)",
	backgroundImage: [
		"radial-gradient(circle at 1px 1px, color-mix(in srgb, var(--accent) 55%, transparent) 1px, transparent 0)",
		"repeating-linear-gradient(135deg, color-mix(in srgb, var(--accent) 28%, transparent) 0 1px, transparent 1px 14px)",
		"repeating-linear-gradient(45deg, color-mix(in srgb, #ffffff 6%, transparent) 0 1px, transparent 1px 28px)",
	].join(", "),
	backgroundSize: "18px 18px, 20px 20px, 40px 40px",
};

const installTheme = {
	"--background": "#0a0a0a",
	"--surface": "#141414",
	"--border": "#262626",
	"--muted": "#a3a3a3",
	"--foreground": "#e5e5e5",
	"--accent": "#8b5cf6",
} as CSSProperties;

export async function PreFooter({ version, tone = "default" }: PreFooterProps) {
	const wingetAvailable = await winget();
	const platforms = supportsWindowsRelease(version)
		? "macOS, Linux, and Windows 11"
		: "macOS and Linux";
	const night = tone === "night";

	return (
		<section className="relative w-full overflow-hidden">
			{night ? (
				<>
					<div
						aria-hidden
						className="absolute inset-0"
						style={nightGradientStyle}
					/>
					<div
						aria-hidden
						className="pointer-events-none absolute inset-0"
						style={nightDotStyle}
					/>
				</>
			) : (
				<div
					aria-hidden
					className="absolute inset-0"
					style={patternStyle}
				/>
			)}
			{night ? null : (
				<div
					aria-hidden
					className="pointer-events-none absolute inset-0"
					style={{
						backgroundImage:
							"linear-gradient(to bottom, rgba(0,0,0,0.35), transparent 30%, transparent 70%, rgba(0,0,0,0.4))",
					}}
				/>
			)}

			<div
				className={`relative mx-auto flex w-full flex-col items-center px-4 py-16 sm:px-6 sm:py-20 ${night ? "max-w-[90rem] xl:px-10" : pageMaxWidthClass}`}
			>
				<h2
					className={
						night
							? "mb-4 text-center text-[1.85rem] font-medium leading-[1.15] tracking-[-0.03em] text-[#e8eef6] sm:text-4xl"
							: "mb-3 text-center text-2xl font-medium tracking-tight text-white sm:text-3xl"
					}
				>
					{night ? "Get into your servers faster, now." : "Install Bast"}
				</h2>
				<p
					className={
						night
							? "mb-10 max-w-md text-center text-sm leading-relaxed text-[#9aa8bc]"
							: "mb-8 max-w-md text-center text-sm leading-relaxed text-white/70"
					}
				>
					{platforms}. Uses native OpenSSH, so your config and keys stay where
					they already are.
				</p>

				<div
					className="w-full max-w-xl"
					style={night ? nightInstallTheme : installTheme}
				>
					<InstallCommand
						version={version}
						wingetAvailable={wingetAvailable}
						controlsPlacement={night ? "above" : "start"}
						platformIcons={night}
						showVersion={!night}
					/>
				</div>
			</div>
		</section>
	);
}
