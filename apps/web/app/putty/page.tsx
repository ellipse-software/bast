import { ComparisonCaseStudyPage } from "@/components/comparison-case-study";
import { getLatestBastVersion } from "@/lib/github";
import { createPageMetadata } from "@/lib/metadata";
import { puttyComparison } from "@/lib/comparisons/putty";

export const metadata = {
  ...createPageMetadata({
    title: puttyComparison.title,
    description: puttyComparison.description,
    path: `/${puttyComparison.slug}`,
  }),
  keywords: puttyComparison.keywords,
};

export default async function PuttyPage() {
  const version = await getLatestBastVersion();
  return (
    <ComparisonCaseStudyPage content={puttyComparison} version={version} />
  );
}
