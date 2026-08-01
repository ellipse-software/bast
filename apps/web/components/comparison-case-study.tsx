import Link from "next/link";

import { BackgroundGrid } from "@/components/background-grid";
import { InstallCommand } from "@/components/install-command";
import { PreFooter } from "@/components/pre-footer";
import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { Code, DocLink } from "@/lib/comparisons/marks";
import type { ComparisonCaseStudy } from "@/lib/comparisons/types";
import { pageMaxWidthClass } from "@/lib/layout";
import { siteUrl } from "@/lib/site";

type ComparisonCaseStudyPageProps = {
  content: ComparisonCaseStudy;
  version?: string | null;
};

export function ComparisonCaseStudyPage({
  content,
  version,
}: ComparisonCaseStudyPageProps) {
  const pageUrl = `${siteUrl}/${content.slug}`;

  const articleJsonLd = {
    "@context": "https://schema.org",
    "@type": "Article",
    headline: content.articleHeadline,
    description: content.articleDescription,
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

  const faqJsonLd = {
    "@context": "https://schema.org",
    "@type": "FAQPage",
    mainEntity: content.faqs.map((item) => ({
      "@type": "Question",
      name: item.q,
      acceptedAnswer: {
        "@type": "Answer",
        text: item.a,
      },
    })),
  };

  return (
    <div className="relative flex min-h-full flex-col">
      <BackgroundGrid />
      <SiteHeader />

      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(articleJsonLd) }}
      />
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(faqJsonLd) }}
      />

      <main
        className={`mx-auto flex w-full ${pageMaxWidthClass} flex-1 flex-col px-4 pb-16 pt-10 sm:px-6 sm:pb-16 sm:pt-12 md:pt-14`}
      >
        <article className="w-full">
          <p className="mb-3 text-sm text-muted">
            <Link
              href="/"
              className="text-foreground/80 transition-colors hover:text-foreground"
            >
              Bast.sh
            </Link>
            <span className="mx-2 text-border">/</span>
            {content.competitorName}
          </p>

          <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
            {content.title}
          </h1>

          <p className="mb-10 max-w-2xl text-base leading-relaxed text-muted sm:text-lg">
            {content.lead}
          </p>

          <section className="mb-14">
            <div className="overflow-x-auto bg-border p-px">
              <table className="w-full min-w-[28rem] border-collapse text-left text-sm">
                <thead>
                  <tr>
                    <th
                      scope="col"
                      className="w-28 border-b border-border bg-background px-4 py-3 font-medium text-muted"
                    >
                      <span className="sr-only">Topic</span>
                    </th>
                    <th
                      scope="col"
                      className="border-b border-border bg-surface px-4 py-3 font-medium text-foreground"
                    >
                      Bast
                    </th>
                    <th
                      scope="col"
                      className="border-b border-border bg-background px-4 py-3 font-medium text-muted"
                    >
                      {content.competitorName}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {content.diffRows.map((row) => (
                    <tr key={row.topic}>
                      <th
                        scope="row"
                        className="border-b border-border bg-background px-4 py-3.5 align-top font-medium text-foreground"
                      >
                        {row.topic}
                      </th>
                      <td className="border-b border-border bg-surface px-4 py-3.5 align-top leading-relaxed text-foreground">
                        {row.bast}
                      </td>
                      <td className="border-b border-border bg-background px-4 py-3.5 align-top leading-relaxed text-muted">
                        {row.competitor}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

          {content.sections.map((section) => (
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

          <section className="mb-12 space-y-4 text-sm leading-relaxed text-muted sm:text-[15px]">
            <h2 className="text-xl font-medium tracking-tight text-foreground">
              {content.whenBetterTitle}
            </h2>
            <p>{content.whenBetterIntro}</p>
            <ul className="list-disc space-y-2 pl-5">
              {content.whenBetterItems.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
            <p>{content.whenBetterOutro}</p>
          </section>

          <section className="mb-14 space-y-4 text-sm leading-relaxed text-muted sm:text-[15px]">
            <h2 className="text-xl font-medium tracking-tight text-foreground">
              {content.migrateTitle}
            </h2>
            <ol className="list-decimal space-y-3 pl-5">
              {content.migrateSteps.map((step, index) => (
                <li key={index}>{step}</li>
              ))}
            </ol>
          </section>

          <section className="mb-6">
            <div className="bg-border p-px">
              <div className="bg-background px-4 py-5 sm:px-6 sm:py-6">
                <h2 className="mb-2 text-lg font-medium tracking-tight text-foreground">
                  Install Bast
                </h2>
                <p className="mb-5 text-sm leading-relaxed text-muted">
                  One command. Then run <Code>bast</Code> and connect with the
                  OpenSSH you already trust.
                </p>
                <InstallCommand version={version} className="w-full" />
                <p className="mt-5 text-sm text-muted">
                  Prefer the short version? See the{" "}
                  <DocLink href="/">Bast homepage</DocLink> or read the{" "}
                  <DocLink href="/docs">docs</DocLink>.
                </p>
              </div>
            </div>
          </section>

          {content.related.length > 0 ? (
            <section className="mb-10">
              <h2 className="mb-3 text-sm font-medium tracking-tight text-foreground">
                More comparisons
              </h2>
              <ul className="flex flex-wrap gap-x-4 gap-y-2 text-sm text-muted">
                {content.related.map((item) => (
                  <li key={item.href}>
                    <Link
                      href={item.href}
                      className="text-foreground/80 underline-offset-2 hover:text-foreground hover:underline"
                    >
                      {item.label}
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}

          <section className="mb-4">
            <div className="bg-border p-px">
              <div className="divide-y divide-border bg-background">
                {content.faqs.map((item) => (
                  <details key={item.q} className="group">
                    <summary className="flex cursor-pointer list-none items-center justify-between gap-4 px-4 py-4 text-sm font-medium tracking-tight marker:content-none [&::-webkit-details-marker]:hidden">
                      <span>{item.q}</span>
                      <span
                        aria-hidden
                        className="shrink-0 text-muted transition-transform group-open:rotate-45"
                      >
                        +
                      </span>
                    </summary>
                    <div className="px-4 pb-4 text-sm leading-relaxed text-muted">
                      {item.a}
                    </div>
                  </details>
                ))}
              </div>
            </div>
          </section>
        </article>
      </main>

      <PreFooter version={version} />
      <SiteFooter />
    </div>
  );
}
