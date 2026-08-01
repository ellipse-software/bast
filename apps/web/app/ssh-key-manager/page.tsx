import { GuidePageView } from "@/components/guide-page";
import { getLatestBastVersion } from "@/lib/github";
import { sshKeyManagerGuide } from "@/lib/guides/ssh-key-manager";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = {
  ...createPageMetadata({
    title: sshKeyManagerGuide.title,
    description: sshKeyManagerGuide.description,
    path: `/${sshKeyManagerGuide.slug}`,
  }),
  keywords: sshKeyManagerGuide.keywords,
};

export default async function SshKeyManagerPage() {
  const version = await getLatestBastVersion();
  return <GuidePageView content={sshKeyManagerGuide} version={version} />;
}
