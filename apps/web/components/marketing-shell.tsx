import Link from "next/link";
import type { ReactNode } from "react";

import { BackgroundGrid } from "@/components/background-grid";
import { PreFooter } from "@/components/pre-footer";
import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { pageMaxWidthClass } from "@/lib/layout";

type MarketingShellProps = {
  children: ReactNode;
  version?: string | null;
};

export function MarketingShell({ children, version }: MarketingShellProps) {
  return (
    <div className="relative flex min-h-full flex-col">
      <BackgroundGrid />
      <SiteHeader />
      <main
        className={`mx-auto flex w-full ${pageMaxWidthClass} flex-1 flex-col px-4 pb-16 pt-10 sm:px-6 sm:pb-16 sm:pt-12 md:pt-14`}
      >
        <div className="w-full">{children}</div>
      </main>
      <PreFooter version={version} />
      <SiteFooter />
    </div>
  );
}

export function MarketingBreadcrumb({
  label,
  parentHref,
  parentLabel,
}: {
  label: string;
  parentHref?: string;
  parentLabel?: string;
}) {
  return (
    <p className="mb-3 text-sm text-muted">
      <Link
        href="/"
        className="text-foreground/80 transition-colors hover:text-foreground"
      >
        Bast.sh
      </Link>
      {parentHref && parentLabel ? (
        <>
          <span className="mx-2 text-border">/</span>
          <Link
            href={parentHref}
            className="text-foreground/80 transition-colors hover:text-foreground"
          >
            {parentLabel}
          </Link>
        </>
      ) : null}
      <span className="mx-2 text-border">/</span>
      {label}
    </p>
  );
}
