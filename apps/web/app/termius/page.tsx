import { ComparisonCaseStudyPage } from "@/components/comparison-case-study";
import { getLatestBastVersion } from "@/lib/github";
import { createPageMetadata } from "@/lib/metadata";
import { termiusComparison } from "@/lib/comparisons/termius";

export const metadata = {
  ...createPageMetadata({
    title: termiusComparison.title,
    description: termiusComparison.description,
    path: `/${termiusComparison.slug}`,
  }),
  keywords: termiusComparison.keywords,
};

export default async function TermiusPage() {
  const version = await getLatestBastVersion();
  return (
    <ComparisonCaseStudyPage content={termiusComparison} version={version} />
  );
}
