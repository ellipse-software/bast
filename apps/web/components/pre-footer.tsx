import type { CSSProperties } from "react";

import { InstallCommand } from "@/components/install-command";
import { pageMaxWidthClass } from "@/lib/layout";

type PreFooterProps = {
  version?: string | null;
};

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

export function PreFooter({ version }: PreFooterProps) {
  return (
    <section className="relative w-full overflow-hidden">
      <div aria-hidden className="absolute inset-0" style={patternStyle} />
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          backgroundImage:
            "linear-gradient(to bottom, rgba(0,0,0,0.35), transparent 30%, transparent 70%, rgba(0,0,0,0.4))",
        }}
      />

      <div
        className={`relative mx-auto flex w-full ${pageMaxWidthClass} flex-col items-center px-4 py-16 sm:px-6 sm:py-20`}
      >
        <h2 className="mb-3 text-center text-2xl font-medium tracking-tight text-white sm:text-3xl">
          Install Bast
        </h2>
        <p className="mb-8 max-w-md text-center text-sm leading-relaxed text-white/70">
          macOS, Linux, and Windows 11. Uses native OpenSSH, so your config and
          keys stay where they already are.
        </p>

        <div className="w-full max-w-xl" style={installTheme}>
          <InstallCommand version={version} />
        </div>
      </div>
    </section>
  );
}
