import { MarketingBreadcrumb, MarketingShell } from "@/components/marketing-shell";
import { aboutPage } from "@/lib/trust-pages";
import { createPageMetadata } from "@/lib/metadata";
import { aboutPath } from "@/lib/company";
import { getLatestBastVersion } from "@/lib/github";

export const metadata = createPageMetadata({
  title: aboutPage.title,
  description: aboutPage.description,
  path: aboutPath,
});

export default async function AboutPage() {
  const version = await getLatestBastVersion();
  return (
    <MarketingShell version={version} preFooter={false}>
      <MarketingBreadcrumb label="About" />
      <article className="w-full">
        <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
          {aboutPage.title}
        </h1>
        <div className="max-w-2xl space-y-4 text-sm leading-relaxed text-muted sm:text-[15px]">
          {aboutPage.paragraphs.map((paragraph) => (
            <p key={paragraph.slice(0, 48)}>{paragraph}</p>
          ))}
        </div>
      </article>
    </MarketingShell>
  );
}
