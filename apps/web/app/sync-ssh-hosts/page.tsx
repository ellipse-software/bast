import { GuidePageView } from "@/components/guide-page";
import { getLatestBastVersion } from "@/lib/github";
import { syncSshHostsGuide } from "@/lib/guides/sync-ssh-hosts";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = {
  ...createPageMetadata({
    title: syncSshHostsGuide.title,
    description: syncSshHostsGuide.description,
    path: `/${syncSshHostsGuide.slug}`,
  }),
  keywords: syncSshHostsGuide.keywords,
};

export default async function SyncSshHostsPage() {
  const version = await getLatestBastVersion();
  return <GuidePageView content={syncSshHostsGuide} version={version} />;
}
