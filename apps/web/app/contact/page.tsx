import { MarketingBreadcrumb, MarketingShell } from "@/components/marketing-shell";
import { contactPage } from "@/lib/trust-pages";
import { createPageMetadata } from "@/lib/metadata";
import { company, contactPath } from "@/lib/company";
import { getLatestBastVersion } from "@/lib/github";
import { legalLinkClass } from "@/components/legal-page";

export const metadata = createPageMetadata({
  title: contactPage.title,
  description: contactPage.description,
  path: contactPath,
});

export default async function ContactPage() {
  const version = await getLatestBastVersion();
  return (
    <MarketingShell version={version} preFooter={false}>
      <MarketingBreadcrumb label="Contact" />
      <article className="w-full">
        <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
          {contactPage.title}
        </h1>
        <div className="mb-10 max-w-2xl space-y-4 text-sm leading-relaxed text-muted sm:text-[15px]">
          {contactPage.paragraphs.map((paragraph) => (
            <p key={paragraph.slice(0, 48)}>{paragraph}</p>
          ))}
        </div>
        <dl className="max-w-xl divide-y divide-border border-y border-border text-sm">
          <div className="flex flex-col gap-1 py-4 sm:flex-row sm:gap-8">
            <dt className="w-28 shrink-0 font-medium tracking-tight text-foreground">
              Legal
            </dt>
            <dd>
              <a className={legalLinkClass} href={`mailto:${company.legalEmail}`}>
                {company.legalEmail}
              </a>
            </dd>
          </div>
          <div className="flex flex-col gap-1 py-4 sm:flex-row sm:gap-8">
            <dt className="w-28 shrink-0 font-medium tracking-tight text-foreground">
              Privacy
            </dt>
            <dd>
              <a className={legalLinkClass} href={`mailto:${company.privacyEmail}`}>
                {company.privacyEmail}
              </a>
            </dd>
          </div>
          <div className="flex flex-col gap-1 py-4 sm:flex-row sm:gap-8">
            <dt className="w-28 shrink-0 font-medium tracking-tight text-foreground">
              Post
            </dt>
            <dd>
              {company.legalName}
              <br />
              {company.registeredAddress}
            </dd>
          </div>
        </dl>
      </article>
    </MarketingShell>
  );
}
