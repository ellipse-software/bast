import Image from "next/image";

import { BackgroundGrid } from "@/components/background-grid";
import { Comparison } from "@/components/comparison";
import { Faq } from "@/components/faq";
import { Sponsors } from "@/components/sponsors";
import { InstallCommand } from "@/components/install-command";
import { PreFooter } from "@/components/pre-footer";
import { Showcase } from "@/components/showcase";
import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { Testimonials } from "@/components/testimonials";
import { getLatestBastVersion } from "@/lib/github";
import { homeHeadline, homeLead, homeSecondary } from "@/lib/home";
import { pageMaxWidthClass } from "@/lib/layout";
import { winget } from "@/flags";

export default async function Home() {
  const [version, wingetAvailable] = await Promise.all([
    getLatestBastVersion(),
    winget(),
  ]);
  return (
    <div className="relative flex min-h-full flex-col">
      <BackgroundGrid accentWash />
      <SiteHeader />
      <main className="flex flex-1 flex-col">
        <div
          className={`mx-auto flex w-full ${pageMaxWidthClass} flex-col items-center px-4 pt-12 sm:px-6 sm:pt-14 md:pt-16 lg:pt-20`}
        >
          <header className="mb-12 flex w-full flex-col items-center text-center sm:mb-10">
            <h1 className="mb-8 w-full max-w-3xl sm:mb-10">
              <span className="sr-only">{homeHeadline}</span>
              <Image
                src="/bast-long-word.svg"
                alt=""
                width={961}
                height={140}
                unoptimized
                className="h-auto w-full"
                sizes="(max-width: 48rem) 100vw, 48rem"
                priority
              />
            </h1>
            <p className="max-w-lg text-base leading-relaxed text-muted sm:text-base md:text-lg">
              {homeLead}
            </p>
            <p className="sr-only">{homeSecondary}</p>
          </header>

          <section className="mb-16 flex w-full flex-col items-center sm:mb-14">
            <InstallCommand
              version={version}
              wingetAvailable={wingetAvailable}
            />
          </section>

          <section className="mb-16 flex w-full flex-col items-center sm:mb-14">
            <Showcase />
          </section>
        </div>

        <section className="mb-16 w-full sm:mb-14">
          <Testimonials />
        </section>

        <div
          className={`mx-auto flex w-full ${pageMaxWidthClass} flex-col items-center px-4 pb-16 sm:px-6 sm:pb-16`}
        >
          <section className="mb-16 flex w-full flex-col items-center sm:mb-14">
            <Comparison />
          </section>

          <section className="mb-16 flex w-full flex-col items-center sm:mb-14">
            <Faq />
          </section>

          <section className="mb-4 flex w-full flex-col items-center sm:mb-2">
            <Sponsors />
          </section>
        </div>
      </main>

      <PreFooter version={version} />
      <SiteFooter />
    </div>
  );
}
