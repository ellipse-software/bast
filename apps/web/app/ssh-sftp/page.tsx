import { GuidePageView } from "@/components/guide-page";
import { getLatestBastVersion } from "@/lib/github";
import { sshSftpGuide } from "@/lib/guides/ssh-sftp";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = {
  ...createPageMetadata({
    title: sshSftpGuide.title,
    description: sshSftpGuide.description,
    path: `/${sshSftpGuide.slug}`,
  }),
  keywords: sshSftpGuide.keywords,
};

export default async function SshSftpPage() {
  const version = await getLatestBastVersion();
  return <GuidePageView content={sshSftpGuide} version={version} />;
}
