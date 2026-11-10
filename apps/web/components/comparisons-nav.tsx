"use client";

import { NavDropdown } from "@/components/nav-dropdown";
import { comparisonNavItems } from "@/lib/marketing";

export function ComparisonsNav({
  tone = "default",
}: {
  tone?: "default" | "night";
}) {
  return (
    <NavDropdown
      label="Comparisons"
      href="/alternatives"
      menuLabel="Comparisons"
      toggleLabel="Open comparisons menu"
      tone={tone}
      overview={{
        href: "/alternatives",
        label: "All comparisons",
        blurb: "Bast vs the field",
      }}
      items={comparisonNavItems.map((item) => ({
        href: item.href,
        label: `vs ${item.label}`,
        blurb: item.blurb,
      }))}
    />
  );
}
