import Link from "next/link";

import {
  MarketingBreadcrumb,
  MarketingShell,
} from "@/components/marketing-shell";
import { getLatestBastVersion } from "@/lib/github";
import { guideNavItems } from "@/lib/marketing";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = {
  ...createPageMetadata({
    title: "Bast features",
    description:
      "Bast features for terminal-native SSH: host management, encrypted vault sync, cloud VM import, SFTP, and OpenSSH key management.",
    path: "/features",
  }),
  keywords: [
    "Bast features",
    "SSH host manager",
    "SSH vault sync",
    "cloud SSH",
    "SFTP TUI",
    "SSH key manager",
    "Bast.sh",
  ],
};

export default async function FeaturesPage() {
  const version = await getLatestBastVersion();

  return (
    <MarketingShell version={version}>
      <MarketingBreadcrumb label="Features" />
      <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
        Bast.sh features
      </h1>
      <p className="mb-10 max-w-2xl text-base leading-relaxed text-muted sm:text-lg">
        Bast stays in the terminal and keeps OpenSSH in charge. These guides
        cover the jobs people actually hire it for.
      </p>

      <section className="mb-10">
        <div className="bg-border p-px">
          <div className="divide-y divide-border bg-background">
            {guideNavItems.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className="block px-4 py-4 transition-colors hover:bg-surface sm:px-5"
              >
                <h2 className="mb-1 text-base font-medium tracking-tight text-foreground">
                  {item.label}
                </h2>
                <p className="text-sm leading-relaxed text-muted">{item.blurb}</p>
              </Link>
            ))}
          </div>
        </div>
      </section>

      <p className="text-sm text-muted">
        Prefer reference docs? Start at the{" "}
        <Link
          href="/docs"
          className="text-foreground underline-offset-2 hover:underline"
        >
          documentation
        </Link>
        .
      </p>
    </MarketingShell>
  );
}
