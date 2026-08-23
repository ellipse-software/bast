import Link from "next/link";

import { MarketingBreadcrumb, MarketingShell } from "@/components/marketing-shell";
import { markdownNotFound } from "@/lib/page-markdown";
import { llmsTxtUrl, openApiUrl, sitemapUrl } from "@/lib/site";

const recovery = [
  { href: "/", label: "Home" },
  { href: "/docs", label: "Docs" },
  { href: llmsTxtUrl, label: "llms.txt" },
  { href: openApiUrl, label: "OpenAPI" },
  { href: sitemapUrl, label: "Sitemap" },
  { href: "/developers", label: "Developers" },
  { href: "/cli", label: "CLI" },
] as const;

export default function NotFound() {
  return (
    <MarketingShell preFooter={false}>
      <MarketingBreadcrumb label="Not found" />
      <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
        Page not found
      </h1>
      <p className="mb-8 max-w-xl text-base leading-relaxed text-muted sm:text-lg">
        This path does not exist on Bast.sh. Use the sitemap, docs index, or
        OpenAPI spec to find a real URL.
      </p>
      <ul className="divide-y divide-border border-y border-border text-sm">
        {recovery.map((item) => (
          <li key={item.href}>
            {item.href.startsWith("http") ? (
              <a
                href={item.href}
                className="block py-3 text-foreground/80 transition-colors hover:text-foreground"
              >
                {item.label}
              </a>
            ) : (
              <Link
                href={item.href}
                className="block py-3 text-foreground/80 transition-colors hover:text-foreground"
              >
                {item.label}
              </Link>
            )}
          </li>
        ))}
      </ul>
      <pre className="sr-only">{markdownNotFound("/not-found")}</pre>
    </MarketingShell>
  );
}
