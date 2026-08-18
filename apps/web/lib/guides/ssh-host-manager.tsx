import { Code, DocLink } from "@/lib/comparisons/marks";
import type { GuidePage } from "@/lib/guides/types";

export const sshHostManagerGuide: GuidePage = {
  slug: "ssh-host-manager",
  title: "SSH host manager for the terminal",
  description:
    "Manage SSH hosts from the terminal without leaving OpenSSH. Bast is an SSH host manager that browses ~/.ssh/config, adds groups and tags, and connects with your system ssh binary.",
  keywords: [
    "SSH host manager",
    "SSH config manager",
    "manage SSH hosts",
    "OpenSSH host picker",
    "Bast.sh",
  ],
  lead: "If your host list lives in notes, shell history, or a GUI database, connecting is slower than it should be. Bast is an SSH host manager that keeps OpenSSH as the source of truth.",
  problemTitle: "The usual mess",
  problem: [
    <>
      Most people accumulate SSH destinations in three places at once:{" "}
      <Code>~/.ssh/config</Code>, half-remembered aliases, and whatever GUI
      client they used last month. Searching is slow. Labels drift. Jump hosts
      get retyped.
    </>,
    <>
      Dedicated GUI host managers fix discovery by inventing a second database.
      Then your scripts, editors, and teammates no longer share the same
      inventory.
    </>,
  ],
  solutionTitle: "What Bast does instead",
  solution: [
    <>
      Bast reads the SSH config you already have, including{" "}
      <Code>Include</Code> files. It adds organization on top: groups, tags,
      colors, notes, favorites, and smart sorting. Connection settings stay in
      OpenSSH.
    </>,
    <>
      Pick a host in the TUI or run <Code>bast &quot;Production web&quot;</Code>{" "}
      from the shell. Bast launches your system <Code>ssh</Code> binary. No
      proprietary protocol, no parallel host store.
    </>,
  ],
  stepsTitle: "Get a clean host list in minutes",
  steps: [
    <>
      Install Bast, then run <Code>bast</Code> to open the host picker.
    </>,
    <>
      Confirm existing hosts appear from <Code>~/.ssh/config</Code>. Add labels,
      groups, and tags for the ones you reach every day.
    </>,
    <>
      Optionally import destinations from shell history, then connect with Enter
      or <Code>bast &lt;label&gt;</Code>.
    </>,
  ],
  sections: [
    {
      title: "Built for people who already use OpenSSH",
      paragraphs: [
        <>
          Bast runs in macOS, Linux, and Windows 11 terminals. If you want a
          phone-native GUI client or a broader Windows desktop suite, see the{" "}
          <DocLink href="/alternatives">comparisons</DocLink>. If you want a
          faster path through the config you already trust, Bast is the host
          manager that fits.
        </>,
      ],
    },
  ],
};
