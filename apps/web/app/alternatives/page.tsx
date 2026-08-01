import Link from "next/link";

import {
  MarketingBreadcrumb,
  MarketingShell,
} from "@/components/marketing-shell";
import { getLatestBastVersion } from "@/lib/github";
import { comparisonNavItems, guideNavItems } from "@/lib/marketing";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = {
  ...createPageMetadata({
    title: "Bast alternatives and comparisons",
    description:
      "Compare Bast to Termius, PuTTY, MobaXterm, and SecureCRT. Bast is the terminal-native OpenSSH host manager for people who want speed without giving up ~/.ssh/config.",
    path: "/alternatives",
  }),
  keywords: [
    "Bast alternatives",
    "Termius alternative",
    "PuTTY alternative",
    "MobaXterm alternative",
    "SecureCRT alternative",
    "OpenSSH host manager",
    "Bast.sh",
  ],
};

export default async function AlternativesPage() {
  const version = await getLatestBastVersion();

  return (
    <MarketingShell version={version}>
      <MarketingBreadcrumb label="Comparisons" />
      <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
        Bast comparisons
      </h1>
      <p className="mb-10 max-w-2xl text-base leading-relaxed text-muted sm:text-lg">
        Bast is for people who already live in OpenSSH. These write-ups explain
        when Bast is the better fit than common GUI clients, and when it is not.
      </p>

      <section className="mb-14">
        <div className="bg-border p-px">
          <div className="grid gap-px bg-border sm:grid-cols-2">
            {comparisonNavItems.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className="bg-background p-5 transition-colors hover:bg-surface sm:p-6"
              >
                <h2 className="mb-1 text-base font-medium tracking-tight text-foreground">
                  Bast vs {item.label}
                </h2>
                <p className="text-sm leading-relaxed text-muted">
                  {item.blurb}. Deep dive on OpenSSH, host storage, sync, and
                  when to keep {item.label}.
                </p>
              </Link>
            ))}
          </div>
        </div>
      </section>

      <section className="mb-4">
        <h2 className="mb-3 text-lg font-medium tracking-tight text-foreground">
          Problem guides
        </h2>
        <p className="mb-5 text-sm leading-relaxed text-muted">
          Looking for a job-to-be-done instead of a brand comparison?
        </p>
        <ul className="divide-y divide-border border border-border bg-background">
          {guideNavItems.map((item) => (
            <li key={item.href}>
              <Link
                href={item.href}
                className="flex items-center justify-between gap-4 px-4 py-3.5 text-sm text-foreground/90 transition-colors hover:bg-surface hover:text-foreground"
              >
                <span>{item.label}</span>
                <span aria-hidden className="text-muted">
                  →
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </section>
    </MarketingShell>
  );
}
