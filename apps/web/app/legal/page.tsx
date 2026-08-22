import Link from "next/link";

import { MarketingBreadcrumb } from "@/components/marketing-shell";
import { company, privacyPath, termsPath } from "@/lib/company";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = createPageMetadata({
  title: "Legal",
  description: `Legal information for Bast.sh, a product of ${company.legalName} trading as ${company.tradingName}.`,
  path: "/legal",
});

export default function LegalIndexPage() {
  return (
    <>
      <MarketingBreadcrumb label="Legal" />
      <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
        Legal
      </h1>
      <p className="mb-10 text-base leading-relaxed text-muted sm:text-lg">
        Bast.sh is a product and hosted service of {company.legalName} trading
        as {company.tradingName}.
      </p>
      <ul className="divide-y divide-border border-y border-border text-sm">
        <li>
          <Link
            href={privacyPath}
            className="block py-4 font-medium tracking-tight transition-colors hover:text-muted"
          >
            Privacy Policy
          </Link>
        </li>
        <li>
          <Link
            href={termsPath}
            className="block py-4 font-medium tracking-tight transition-colors hover:text-muted"
          >
            Terms of Service
          </Link>
        </li>
      </ul>
    </>
  );
}
