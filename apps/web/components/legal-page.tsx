import type { ReactNode } from "react";

import { MarketingBreadcrumb } from "@/components/marketing-shell";

export const legalLinkClass =
  "text-foreground underline-offset-2 hover:underline";

export function LegalPage({
  title,
  updated,
  children,
}: {
  title: string;
  updated: string;
  children: ReactNode;
}) {
  return (
    <>
      <MarketingBreadcrumb label={title} parentHref="/legal" parentLabel="Legal" />
      <article className="w-full">
        <h1 className="mb-3 text-3xl font-medium tracking-tight sm:text-4xl">
          {title}
        </h1>
        <p className="mb-10 text-sm text-muted">Effective {updated}</p>
        <div className="space-y-10 text-sm leading-relaxed text-muted sm:text-[15px]">
          {children}
        </div>
      </article>
    </>
  );
}

export function LegalSection({
  id,
  title,
  children,
}: {
  id: string;
  title: string;
  children: ReactNode;
}) {
  return (
    <section id={id} className="scroll-mt-24 space-y-3">
      <h2 className="text-base font-medium tracking-tight text-foreground">
        {title}
      </h2>
      {children}
    </section>
  );
}

export function LegalList({ children }: { children: ReactNode }) {
  return <ul className="ml-5 list-disc space-y-1.5">{children}</ul>;
}
