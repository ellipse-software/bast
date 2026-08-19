import { InstallCommand } from "@/components/install-command";
import {
  MarketingBreadcrumb,
  MarketingShell,
} from "@/components/marketing-shell";
import { Code } from "@/lib/comparisons/marks";
import type { GuidePage } from "@/lib/guides/types";
import { supportsWindowsRelease } from "@/lib/releases";
import { siteUrl } from "@/lib/site";

type GuidePageViewProps = {
  content: GuidePage;
  version?: string | null;
};

export function GuidePageView({ content, version }: GuidePageViewProps) {
  const pageUrl = `${siteUrl}/${content.slug}`;
  const platforms = supportsWindowsRelease(version)
    ? "macOS, Linux, and Windows 11"
    : "macOS and Linux";

  const articleJsonLd = {
    "@context": "https://schema.org",
    "@type": "Article",
    headline: content.title,
    description: content.description,
    author: {
      "@type": "Organization",
      name: "Bast.sh",
      url: siteUrl,
    },
    publisher: {
      "@type": "Organization",
      name: "Bast.sh",
      url: siteUrl,
    },
    mainEntityOfPage: pageUrl,
    url: pageUrl,
  };

  return (
    <MarketingShell version={version}>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(articleJsonLd) }}
      />

      <MarketingBreadcrumb label={content.title} />

      <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
        {content.title}
      </h1>
      <p className="mb-10 max-w-2xl text-base leading-relaxed text-muted sm:text-lg">
        {content.lead}
      </p>

      <section className="mb-12 space-y-4 text-sm leading-relaxed text-muted sm:text-[15px]">
        <h2 className="text-xl font-medium tracking-tight text-foreground">
          {content.problemTitle}
        </h2>
        {content.problem.map((paragraph, index) => (
          <p key={`problem-${index}`}>{paragraph}</p>
        ))}
      </section>

      <section className="mb-12 space-y-4 text-sm leading-relaxed text-muted sm:text-[15px]">
        <h2 className="text-xl font-medium tracking-tight text-foreground">
          {content.solutionTitle}
        </h2>
        {content.solution.map((paragraph, index) => (
          <p key={`solution-${index}`}>{paragraph}</p>
        ))}
      </section>

      <section className="mb-12 space-y-4 text-sm leading-relaxed text-muted sm:text-[15px]">
        <h2 className="text-xl font-medium tracking-tight text-foreground">
          {content.stepsTitle}
        </h2>
        <ol className="list-decimal space-y-3 pl-5">
          {content.steps.map((step, index) => (
            <li key={`step-${index}`}>{step}</li>
          ))}
        </ol>
      </section>

      {content.sections?.map((section) => (
        <section
          key={section.title}
          className="mb-12 space-y-4 text-sm leading-relaxed text-muted sm:text-[15px]"
        >
          <h2 className="text-xl font-medium tracking-tight text-foreground">
            {section.title}
          </h2>
          {section.paragraphs.map((paragraph, index) => (
            <p key={`${section.title}-${index}`}>{paragraph}</p>
          ))}
        </section>
      ))}

      <section className="mb-4">
        <div className="bg-border p-px">
          <div className="bg-background px-4 py-5 sm:px-6 sm:py-6">
            <h2 className="mb-2 text-lg font-medium tracking-tight text-foreground">
              Install Bast
            </h2>
            <p className="mb-5 text-sm leading-relaxed text-muted">
              {platforms}. Then run <Code>bast</Code> and work from your existing
              OpenSSH setup.
            </p>
            <InstallCommand version={version} className="w-full" />
          </div>
        </div>
      </section>
    </MarketingShell>
  );
}
