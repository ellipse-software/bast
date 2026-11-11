import type { Metadata } from "next";
import type { CSSProperties } from "react";

import { BeachDither } from "@/components/beach-dither";
import { Comparison } from "@/components/comparison";
import { FeatureSection } from "@/components/new-features";
import { InstallCommand } from "@/components/install-command";
import { PreFooter } from "@/components/pre-footer";
import { ProviderSection } from "@/components/provider-marks";
import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { Sponsors } from "@/components/sponsors";
import { Testimonials } from "@/components/testimonials";
import { TuiDemo } from "@/components/tui-demo";
import { winget } from "@/flags";
import { getLatestBastVersion } from "@/lib/github";
import { homeLead, homeTitle } from "@/lib/home";
import { defaultDescription } from "@/lib/metadata";
import { nightSurface } from "@/lib/night";

export const metadata: Metadata = {
  title: {
    absolute: "Bast.sh: native SSH picker and key manager",
  },
  description: defaultDescription,
  alternates: {
    canonical: "/",
  },
};

const installTheme = {
  "--background": "#070b14",
  "--foreground": "#e7eef8",
  "--muted": "#b4c0d2",
  "--border": "rgba(231, 238, 248, 0.22)",
  "--surface": "#05080f",
  "--accent": "#c6b6f2",
} as CSSProperties;

const demoTheme = {
  "--background": "#0c0f14",
  "--foreground": "#E5E7EB",
  "--muted": "#9CA3AF",
  "--border": "#2A3142",
  "--surface": "#1F2937",
  "--accent": "#8B5CF6",
  "--highlight": "#1F2937",
} as CSSProperties;

const titleCut = homeTitle.indexOf(" servers");
const titleFirst = titleCut === -1 ? homeTitle : homeTitle.slice(0, titleCut);
const titleSecond = titleCut === -1 ? null : homeTitle.slice(titleCut + 1);

const leadCut = homeLead.indexOf(", manage keys");
const leadFirst = leadCut === -1 ? homeLead : homeLead.slice(0, leadCut + 1);
const leadSecond = leadCut === -1 ? null : homeLead.slice(leadCut + 2);

export default async function Home() {
  const [version, wingetAvailable] = await Promise.all([
    getLatestBastVersion(),
    winget(),
  ]);

  return (
    <>
    <section className="relative flex min-h-dvh flex-col bg-[#020814] text-[#e8eef6] selection:bg-[#24345f] selection:text-white">
      <BeachDither />
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          background:
            "linear-gradient(to top, rgba(3,5,14,0.78) 0%, rgba(3,5,14,0.62) 24%, rgba(3,5,14,0.28) 42%, transparent 58%)",
        }}
      />
      <div
        aria-hidden
        className="pointer-events-none absolute inset-x-0 top-0 h-32"
        style={{
          background:
            "linear-gradient(to bottom, rgba(2,8,20,0.9) 0%, rgba(2,8,20,0.55) 46%, transparent 100%)",
        }}
      />
      <style>{`
        @media (min-width: 80rem) {
          .new-demo-frame.new-demo-frame {
            width: 72rem;
            height: min(680px, calc(100dvh - 11rem));
            transform: translate3d(33rem, -50%, 0);
          }
        }
        @media (min-width: 90rem) {
          .new-demo-frame.new-demo-frame {
            /* Left stays 48rem into the centered 90rem column. Width grows so the right edge stays 30rem past the viewport. */
            right: auto;
            left: calc((100% - 90rem) / 2 + 48rem);
            width: calc(50% + 27rem);
            transform: translate3d(0, -50%, 0);
          }
        }
      `}</style>
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="new-demo-frame pointer-events-auto absolute hidden xl:inset-x-auto xl:right-0 xl:top-1/2 xl:block xl:min-h-0">
          <div
            className="absolute inset-0 flex flex-col overflow-hidden rounded-xl border border-white/15 bg-[#0c0f14] text-[#E5E7EB] shadow-[0_28px_64px_-28px_rgba(0,0,0,0.72)]"
            style={demoTheme}
          >
            <div className="relative flex h-10 shrink-0 items-center border-b border-white/10 bg-[#141820] px-3.5">
              <div className="flex items-center gap-2" aria-hidden>
                <span className="size-3 rounded-full bg-[#ff5f57]" />
                <span className="size-3 rounded-full bg-[#febc2e]" />
                <span className="size-3 rounded-full bg-[#28c840]" />
              </div>
              <span className="pointer-events-none absolute inset-x-0 text-center font-mono text-[12px] text-[#9CA3AF]">
                bast
              </span>
            </div>
            <div className="relative min-h-0 flex-1">
              <TuiDemo showLabel={false} showHints={false} roomy />
            </div>
          </div>
        </div>
      </div>

      <div className="pointer-events-none relative z-10 mx-auto flex min-h-dvh w-full max-w-[90rem] flex-col">
        <div className="pointer-events-auto">
          <SiteHeader wide tone="night" />
        </div>
        <div className="pointer-events-auto mt-auto w-fit max-w-full px-4 pb-20 pt-4 sm:px-6 xl:px-10">
          <h1 className="text-[2.05rem] font-medium leading-[1.05] tracking-[-0.03em] text-[#e8eef6] sm:text-[2.5rem] xl:text-[2.85rem]">
            <span className="block">{titleFirst}</span>
            {titleSecond ? <span className="block">{titleSecond}</span> : null}
          </h1>
          <p className="mt-4 text-[13px] leading-snug text-[#c9d5e6] sm:text-[15px] xl:text-base">
            <span className="block">{leadFirst}</span>
            {leadSecond ? <span className="block">{leadSecond}</span> : null}
          </p>
          <div className="mt-8 max-w-[32rem]" style={installTheme}>
            <InstallCommand
              version={version}
              wingetAvailable={wingetAvailable}
              showVersion={false}
              platformIcons
              controlsPlacement="above"
              className="w-full max-w-[32rem]"
            />
          </div>
        </div>
      </div>
    </section>
    <ProviderSection />
    <FeatureSection />
    <section className="bg-[#05080f] text-[#e8eef6]" style={nightSurface}>
      <div className="mx-auto w-full max-w-[90rem] px-4 pt-6 pb-16 sm:px-6 xl:px-10">
        <Comparison tone="night" />
      </div>
    </section>
    <div className="bg-[#05080f] text-[#e8eef6]" style={nightSurface}>
      <Testimonials tone="night" />
    </div>
    <section className="bg-[#05080f] text-[#e8eef6]" style={nightSurface}>
      <div className="mx-auto w-full max-w-[90rem] px-4 py-16 sm:px-6 xl:px-10">
        <Sponsors />
      </div>
    </section>
    <PreFooter version={version} tone="night" />
    <SiteFooter tone="night" />
    </>
  );
}
