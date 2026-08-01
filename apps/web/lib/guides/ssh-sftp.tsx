import { Code, DocLink } from "@/lib/comparisons/marks";
import type { GuidePage } from "@/lib/guides/types";

export const sshSftpGuide: GuidePage = {
  slug: "ssh-sftp",
  title: "SFTP in the terminal with your SSH config",
  description:
    "Browse and transfer files over SFTP from Bast's dual-pane TUI. Uses the same OpenSSH config as Connect, including ProxyJump and cloud tunnels.",
  keywords: [
    "SSH SFTP",
    "SFTP client terminal",
    "SFTP TUI",
    "OpenSSH SFTP",
    "Bast file transfer",
    "Bast.sh",
  ],
  lead: "File transfer should not require a second app with a second host list. Bast Files is dual-pane SFTP that reuses the SSH config you already connect with.",
  problemTitle: "Hosts for shell, hosts for files, twice the drift",
  problem: [
    <>
      Many workflows split SSH and SFTP across tools. The shell knows one jump
      path. The file app knows another. One gets updated. The other fails at
      6pm.
    </>,
    <>
      GUI SFTP clients often hide that complexity behind paid plans or their own
      session stores.
    </>,
  ],
  solutionTitle: "One config, two panes",
  solution: [
    <>
      Bast Files opens a local and remote browser in the TUI. Copy and move
      files with the same OpenSSH settings Connect already uses, including
      ProxyJump and cloud ProxyCommand tunnels.
    </>,
    <>
      You stay in the terminal. You do not maintain a parallel file-transfer
      address book.
    </>,
  ],
  stepsTitle: "Transfer against a real host",
  steps: [
    <>
      Install Bast and make sure the target host connects normally with{" "}
      <Code>bast</Code> or system <Code>ssh</Code>.
    </>,
    <>
      Open Files from the TUI for that host, browse both panes, and copy what
      you need.
    </>,
    <>
      Read the <DocLink href="/docs/features/files">Files docs</DocLink> for
      keybindings and current transfer limits.
    </>,
  ],
};
