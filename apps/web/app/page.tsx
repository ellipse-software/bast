import { BackgroundGrid } from "@/components/background-grid";
import { AgentResources } from "@/components/ask-ai-menu";
import { Comparison } from "@/components/comparison";
import { Faq } from "@/components/faq";
import { InstallCommand } from "@/components/install-command";
import { PreFooter } from "@/components/pre-footer";
import { Showcase } from "@/components/showcase";
import { SiteHeader } from "@/components/site-header";
import { getLatestBastVersion } from "@/lib/github";
import { llmsTxtUrl } from "@/lib/site";

export default async function Home() {
  const version = await getLatestBastVersion();
  return (
    <div className="relative flex min-h-full flex-col">
      <BackgroundGrid />
      <SiteHeader />
      <main className="mx-auto flex w-full max-w-5xl flex-1 flex-col items-center px-4 pb-16 pt-12 sm:px-6 sm:pb-16 sm:pt-14 md:pt-16 lg:pt-20">
        <header className="mb-12 flex max-w-2xl flex-col items-center text-center sm:mb-10">
          <h1 className="mb-5 text-5xl font-medium tracking-tight sm:mb-4">
            Bast<span className="text-accent">.sh</span>
          </h1>
          <p className="max-w-lg text-base leading-relaxed text-muted sm:text-base md:text-lg">
            Browse SSH hosts, transfer files over SFTP, sync cloud VMs, manage
            keys, and connect from the terminal. The fast way into the servers
            you use every day.
          </p>
        </header>

        <section className="mb-16 flex w-full flex-col items-center sm:mb-14">
          <InstallCommand version={version} />
        </section>

        <section className="mb-16 flex w-full flex-col items-center sm:mb-14">
          <Showcase />
        </section>

        <section className="mb-16 flex w-full flex-col items-center sm:mb-14">
          <Comparison />
        </section>

        <section className="mb-4 flex w-full flex-col items-center sm:mb-2">
          <Faq />
        </section>
      </main>

      <PreFooter version={version} />

      <footer className="mx-auto flex w-full max-w-5xl flex-col items-center gap-4 px-4 py-10 sm:px-6">
        <AgentResources contextUrl={llmsTxtUrl} />
        <p className="text-xs text-muted">
          © {new Date().getFullYear()} Bast.sh
        </p>
      </footer>
    </div>
  );
}
