import { GuidePageView } from "@/components/guide-page";
import { cloudSshGuide } from "@/lib/guides/cloud-ssh";
import { getLatestBastVersion } from "@/lib/github";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = {
  ...createPageMetadata({
    title: cloudSshGuide.title,
    description: cloudSshGuide.description,
    path: `/${cloudSshGuide.slug}`,
  }),
  keywords: cloudSshGuide.keywords,
};

export default async function CloudSshPage() {
  const version = await getLatestBastVersion();
  return <GuidePageView content={cloudSshGuide} version={version} />;
}
