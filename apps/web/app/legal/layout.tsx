import type { ReactNode } from "react";

import { MarketingShell } from "@/components/marketing-shell";

export default function LegalLayout({ children }: { children: ReactNode }) {
  return <MarketingShell preFooter={false}>{children}</MarketingShell>;
}
