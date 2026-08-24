import { Code, DocLink } from "@/lib/comparisons/marks";
import type { ComparisonCaseStudy } from "@/lib/comparisons/types";

export const mobaxtermComparison: ComparisonCaseStudy = {
  slug: "mobaxterm",
  competitorName: "MobaXterm",
  title: "Bast vs MobaXterm",
  description:
    "Bast as a MobaXterm alternative for terminal-native OpenSSH: no Windows toolbox bloat, hosts in ~/.ssh/config, cloud VM sync, SFTP in the TUI, and MIT licensing.",
  keywords: [
    "Bast vs MobaXterm",
    "MobaXterm alternative",
    "MobaXterm alternative Mac",
    "MobaXterm alternative Linux",
    "OpenSSH TUI",
    "Bast.sh",
  ],
  lead: "MobaXterm is a Windows Swiss-army kit: SSH, X11, sniffers, and a pile of network tools in one installer. Bast is narrower on purpose: a terminal-native OpenSSH host picker.",
  articleHeadline:
    "Bast vs MobaXterm: OpenSSH in the terminal instead of a Windows toolbox",
  articleDescription:
    "Why Bast is a sharper MobaXterm alternative when you want native OpenSSH, shared config, and less desktop-suite overhead.",
  diffRows: [
    {
      topic: "Shape",
      bast: "Focused SSH host/key/SFTP TUI",
      competitor: "Broad Windows toolbox (SSH, X11, network utilities)",
    },
    {
      topic: "Platform",
      bast: "macOS, Linux, and Windows 11 terminals",
      competitor: "Windows-first desktop app",
    },
    {
      topic: "SSH stack",
      bast: "System OpenSSH",
      competitor: "Embedded SSH session engine inside MobaXterm",
    },
    {
      topic: "Host storage",
      bast: "~/.ssh/config as source of truth",
      competitor: "Sessions inside the MobaXterm workspace",
    },
    {
      topic: "Cloud hosts",
      bast: "GCP, AWS, Azure, Hetzner Cloud, and box.ascii.dev",
      competitor: "Mostly manual session setup",
    },
    {
      topic: "Licensing",
      bast: "Free MIT open source",
      competitor: "Free Home edition with limits; Professional is paid",
    },
    {
      topic: "Automation",
      bast: "CLI + --json for scripts and agents",
      competitor: "Desktop-oriented workflows",
    },
  ],
  sections: [
    {
      title: "Toolbox vs specialized tool",
      paragraphs: [
        <>
          MobaXterm&apos;s pitch is breadth. One Windows download covers remote
          terminals, file transfer, X11 forwarding, and a long list of network
          utilities. That is useful when your machine is Windows and you want a
          single vendor suite.
        </>,
        <>
          Bast&apos;s pitch is depth on one job: make OpenSSH hosts fast to find,
          organize, and reach without leaving the terminal. It does not try to
          replace Wireshark, an X server, or a general Windows admin console.
        </>,
      ],
    },
    {
      title: "Config you can share with everything else",
      paragraphs: [
        <>
          MobaXterm sessions live inside MobaXterm. That is convenient until you
          need the same inventory in a shell script, a remote editor, or a
          teammate&apos;s pure OpenSSH setup.
        </>,
        <>
          Bast reads and extends <Code>~/.ssh/config</Code>. Hosts stay
          portable. Delete Bast and nothing about your SSH connectivity is
          stranded in a proprietary session store.
        </>,
      ],
    },
    {
      title: "Licensing without edition math",
      paragraphs: [
        <>
          MobaXterm&apos;s Home edition is free with constraints; Professional
          unlocks the broader commercial feature set. For teams, that turns into
          license tracking.
        </>,
        <>
          Bast is MIT-licensed. The host picker, SFTP, cloud import, vault sync
          of Bast-managed state, and JSON CLI are not sitting behind a paid
          edition gate.
        </>,
      ],
    },
    {
      title: "Cloud fleets and agent workflows",
      paragraphs: [
        <>
          Bast can import live hosts from GCP, AWS, Azure, Hetzner Cloud, and box.ascii.dev through the CLIs you
          already authenticate. Synced hosts stay read-only reflections of cloud
          inventory.
        </>,
        <>
          The same binary exposes <Code>bast --json</Code> for scripts and AI
          agents. MobaXterm is built for operators clicking through a Windows
          desktop. Bast is built for operators who type, automate, and stay in
          the shell.
        </>,
      ],
    },
  ],
  whenBetterTitle: "When MobaXterm is still the better choice",
  whenBetterIntro: "Stay on MobaXterm if:",
  whenBetterItems: [
    "You need one Windows suite that also covers X11 and assorted network tools.",
    "Your workplace standardizes on MobaXterm sessions and training.",
    "You want an integrated X server and Unix utility bundle on Windows.",
    "You want a graphical multi-tab Windows terminal more than an OpenSSH-native TUI.",
  ],
  whenBetterOutro:
    "Bast is the better MobaXterm alternative when the real need is OpenSSH host management in the terminal, not a full Windows admin toolkit.",
  migrateTitle: "Switching from MobaXterm to Bast",
  migrateSteps: [
    <>
      Export or rewrite important sessions as OpenSSH config host blocks under{" "}
      <Code>~/.ssh/config</Code>.
    </>,
    <>
      Move keys into standard OpenSSH files if they currently live only inside
      the MobaXterm environment.
    </>,
    <>
      Install Bast on macOS, Linux, or Windows 11 and verify connections with the system{" "}
      <Code>ssh</Code> binary.
    </>,
    <>
      Turn on <DocLink href="/docs/features/vault">Vault</DocLink> and{" "}
      <DocLink href="/docs/features/aws">cloud sync</DocLink> if you want
      encrypted multi-machine state and live cloud inventory.
    </>,
  ],
  faqs: [
    {
      q: "Is Bast a MobaXterm alternative?",
      a: "Yes for terminal-native OpenSSH host management on macOS, Linux, and Windows 11. It is not a replacement for MobaXterm's wider Windows toolbox.",
    },
    {
      q: "Does Bast include X11 and network utilities like MobaXterm?",
      a: "No. Bast focuses on hosts, keys, SFTP, cloud import, and OpenSSH connections.",
    },
    {
      q: "Is Bast free like MobaXterm Home?",
      a: "Bast is free and open source under MIT, without a separate Professional edition for core features.",
    },
    {
      q: "Can I keep using MobaXterm on Windows and Bast elsewhere?",
      a: "Yes. Prefer OpenSSH config as the shared source of truth so both environments can point at the same hosts.",
    },
  ],
  related: [
    { href: "/termius", label: "Bast vs Termius" },
    { href: "/putty", label: "Bast vs PuTTY" },
    { href: "/securecrt", label: "Bast vs SecureCRT" },
  ],
};
