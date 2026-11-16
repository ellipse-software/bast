"use client";

import { NavDropdown } from "@/components/nav-dropdown";
import { guideNavItems } from "@/lib/marketing";

export function FeaturesNav({ tone = "default" }: { tone?: "default" | "night" }) {
  return (
    <NavDropdown
      label="Features"
      href="/features"
      menuLabel="Features"
      toggleLabel="Open features menu"
      tone={tone}
      overview={{
        href: "/features",
        label: "All features",
        blurb: "What Bast is for",
      }}
      items={guideNavItems}
    />
  );
}
