"use client";

import { NavDropdown } from "@/components/nav-dropdown";
import { comparisonNavItems } from "@/lib/marketing";

export function ComparisonsNav() {
  return (
    <NavDropdown
      label="Comparisons"
      href="/alternatives"
      menuLabel="Comparisons"
      toggleLabel="Open comparisons menu"
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
