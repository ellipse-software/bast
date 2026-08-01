import { ComparisonCaseStudyPage } from "@/components/comparison-case-study";
import { getLatestBastVersion } from "@/lib/github";
import { createPageMetadata } from "@/lib/metadata";
import { mobaxtermComparison } from "@/lib/comparisons/mobaxterm";

export const metadata = {
  ...createPageMetadata({
    title: mobaxtermComparison.title,
    description: mobaxtermComparison.description,
    path: `/${mobaxtermComparison.slug}`,
  }),
  keywords: mobaxtermComparison.keywords,
};

export default async function MobaXtermPage() {
  const version = await getLatestBastVersion();
  return (
    <ComparisonCaseStudyPage content={mobaxtermComparison} version={version} />
  );
}
