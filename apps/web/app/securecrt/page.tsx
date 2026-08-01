import { ComparisonCaseStudyPage } from "@/components/comparison-case-study";
import { getLatestBastVersion } from "@/lib/github";
import { createPageMetadata } from "@/lib/metadata";
import { securecrtComparison } from "@/lib/comparisons/securecrt";

export const metadata = {
  ...createPageMetadata({
    title: securecrtComparison.title,
    description: securecrtComparison.description,
    path: `/${securecrtComparison.slug}`,
  }),
  keywords: securecrtComparison.keywords,
};

export default async function SecureCrtPage() {
  const version = await getLatestBastVersion();
  return (
    <ComparisonCaseStudyPage content={securecrtComparison} version={version} />
  );
}
