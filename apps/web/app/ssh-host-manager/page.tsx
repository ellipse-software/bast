import { GuidePageView } from "@/components/guide-page";
import { getLatestBastVersion } from "@/lib/github";
import { sshHostManagerGuide } from "@/lib/guides/ssh-host-manager";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = {
  ...createPageMetadata({
    title: sshHostManagerGuide.title,
    description: sshHostManagerGuide.description,
    path: `/${sshHostManagerGuide.slug}`,
  }),
  keywords: sshHostManagerGuide.keywords,
};

export default async function SshHostManagerPage() {
  const version = await getLatestBastVersion();
  return <GuidePageView content={sshHostManagerGuide} version={version} />;
}
