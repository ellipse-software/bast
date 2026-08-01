import {
  MarketingBreadcrumb,
  MarketingShell,
} from "@/components/marketing-shell";
import { StatusPageView } from "@/components/status-page";
import { getStatusPageData } from "@/lib/betterstack";
import { getLatestBastVersion } from "@/lib/github";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = createPageMetadata({
  title: "Status",
  description:
    "Live Bast.sh service status for the marketing site, docs, and vault, backed by Better Stack monitors.",
  path: "/status",
});

export default async function StatusPage() {
  const [version, data] = await Promise.all([
    getLatestBastVersion(),
    getStatusPageData(),
  ]);

  return (
    <MarketingShell version={version}>
      <MarketingBreadcrumb label="Status" />
      <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
        Status
      </h1>
      <p className="mb-10 max-w-2xl text-base leading-relaxed text-muted sm:text-lg">
        Marketing site, docs, and vault availability.
      </p>
      <StatusPageView data={data} />
    </MarketingShell>
  );
}
