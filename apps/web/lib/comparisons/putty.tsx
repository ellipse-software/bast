import { Code, DocLink } from "@/lib/comparisons/marks";
import type { ComparisonCaseStudy } from "@/lib/comparisons/types";

export const puttyComparison: ComparisonCaseStudy = {
  slug: "putty",
  competitorName: "PuTTY",
  title: "Bast vs PuTTY",
  description:
    "Bast as a modern PuTTY alternative for macOS, Linux, and Windows 11: native OpenSSH, ~/.ssh/config as source of truth, cloud VM sync, SFTP in the TUI, and free MIT licensing.",
  keywords: [
    "Bast vs PuTTY",
    "PuTTY alternative",
    "PuTTY alternative Mac",
    "PuTTY for Linux",
    "OpenSSH host manager",
    "Bast.sh",
  ],
  lead: "PuTTY defined desktop SSH for a generation of Windows operators. Bast is what that job looks like when you live in the terminal and want OpenSSH to stay in charge.",
  articleHeadline:
    "Bast vs PuTTY: a terminal-native alternative for OpenSSH workflows",
  articleDescription:
    "Why Bast is a better fit than PuTTY for developers who want native OpenSSH, shared config, and a fast host picker.",
  diffRows: [
    {
      topic: "Platform",
      bast: "macOS, Linux, and Windows 11 terminals",
      competitor: "Classic Windows desktop app (ports exist elsewhere)",
    },
    {
      topic: "SSH stack",
      bast: "System OpenSSH binary",
      competitor: "PuTTY's own SSH stack and .ppk keys",
    },
    {
      topic: "Sessions",
      bast: "~/.ssh/config plus Bast metadata",
      competitor: "Saved PuTTY sessions in the registry or files",
    },
    {
      topic: "Keys",
      bast: "Standard OpenSSH key files",
      competitor: ".ppk via PuTTYgen; conversion needed for OpenSSH tools",
    },
    {
      topic: "Cloud hosts",
      bast: "CLIs for GCP, AWS, Azure, and Box; APIs for Hetzner, Upstash, and Vercel",
      competitor: "Not built in",
    },
    {
      topic: "File transfers",
      bast: "Dual-pane SFTP in the same TUI",
      competitor: "Separate PSCP / PSFTP tools",
    },
    {
      topic: "Automation",
      bast: "CLI with stable --json output",
      competitor: "Mostly interactive desktop workflows",
    },
    {
      topic: "Price",
      bast: "Free and open source (MIT)",
      competitor: "Free",
    },
  ],
  sections: [
    {
      title: "Different eras, different defaults",
      paragraphs: [
        <>
          PuTTY solved a real problem: Windows needed a trustworthy SSH client
          when OpenSSH was not part of the OS story. Sessions, terminals, and
          key handling lived inside one desktop app, and that model still works
          for a lot of Windows shops.
        </>,
        <>
          Bast starts from the opposite default. OpenSSH is already available
          across its supported platforms. The missing piece is not another SSH implementation. It
          is a fast way to browse, organize, and connect to the hosts your
          config already knows.
        </>,
      ],
    },
    {
      title: "Sessions vs SSH config",
      paragraphs: [
        <>
          PuTTY sessions are a product database. Hostnames, ports, usernames,
          and key choices live in PuTTY&apos;s world. Other tools do not
          automatically share that inventory.
        </>,
        <>
          Bast keeps connection settings in <Code>~/.ssh/config</Code>. That
          means the same hosts work in your shell, VS Code Remote, Cursor,
          Ansible, CI jump boxes, and Bast itself. Presentation metadata
          (groups, tags, notes, colors) sits beside that config instead of
          replacing it.
        </>,
      ],
    },
    {
      title: "Keys without the .ppk detour",
      paragraphs: [
        <>
          PuTTY&apos;s .ppk format made sense inside its ecosystem. It is
          awkward everywhere else. Teams spend real time converting keys so
          OpenSSH tools, cloud metadata, and agent forwarding all agree.
        </>,
        <>
          Bast generates, imports, and installs native OpenSSH keys on disk. No
          proprietary key store, no conversion tax when the rest of your stack
          already speaks OpenSSH.
        </>,
      ],
    },
    {
      title: "One surface for hosts, keys, files, and cloud",
      paragraphs: [
        <>
          With PuTTY you often juggle the session list, PuTTYgen, and separate
          PSCP/PSFTP binaries. Bast keeps the host picker, key manager, dual-pane
          SFTP, and cloud imports in one terminal UI that still hands off
          connections to system <Code>ssh</Code>.
        </>,
        <>
          Cloud sync pulls live hosts from GCP, AWS, Azure, and box.ascii.dev
          through provider CLIs, and from Hetzner Cloud, Upstash Box, and Vercel
          Sandbox over their APIs. That is a different job from saving a static
          PuTTY session for a box you typed in by hand.
        </>,
      ],
    },
  ],
  whenBetterTitle: "When PuTTY is still the better choice",
  whenBetterIntro: "Keep PuTTY (or a Windows-native peer) if:",
  whenBetterItems: [
    "You need a mature graphical Windows SSH client rather than a terminal TUI.",
    "Your org standardizes on PuTTY sessions and .ppk keys as policy.",
    "You only need occasional ad-hoc SSH and do not care about shared OpenSSH config.",
    "You want a classic Windows GUI terminal rather than a terminal-native TUI.",
  ],
  whenBetterOutro:
    "Bast is the OpenSSH-native answer for people who work in the terminal. PuTTY remains the better fit for its established GUI and session ecosystem.",
  migrateTitle: "Moving from PuTTY sessions to Bast",
  migrateSteps: [
    <>
      Convert important .ppk keys to OpenSSH format and place them under{" "}
      <Code>~/.ssh</Code> (or import them with Bast&apos;s key tools).
    </>,
    <>
      Recreate sessions as OpenSSH host blocks in <Code>~/.ssh/config</Code>{" "}
      (Host, HostName, User, Port, IdentityFile, ProxyJump).
    </>,
    <>
      Install Bast on macOS, Linux, or Windows 11. It discovers existing config and adds one
      managed Include for hosts you create inside Bast.
    </>,
    <>
      Use <DocLink href="/docs/features/files">Files</DocLink> for SFTP and{" "}
      <DocLink href="/docs/features/gcp">cloud sync</DocLink> when inventory
      should come from the provider instead of a static session list.
    </>,
  ],
  faqs: [
    {
      q: "Is Bast a PuTTY alternative?",
      a: "Yes for terminal OpenSSH workflows on macOS, Linux, and Windows 11. Bast uses system OpenSSH and ~/.ssh/config instead of PuTTY sessions and .ppk keys.",
    },
    {
      q: "Does Bast run on Windows like PuTTY?",
      a: "Yes on Windows 11. Bast uses Windows OpenSSH and runs in Windows Terminal, PowerShell, or Command Prompt.",
    },
    {
      q: "Can Bast use my old PuTTY keys?",
      a: "After you convert .ppk files to OpenSSH format, Bast can import and use them like any other local key.",
    },
    {
      q: "Why switch from PuTTY to Bast?",
      a: "Shared OpenSSH config, no .ppk conversion tax, cloud VM import, SFTP in the same TUI, and a CLI built for scripts and agents.",
    },
  ],
  related: [
    { href: "/termius", label: "Bast vs Termius" },
    { href: "/mobaxterm", label: "Bast vs MobaXterm" },
    { href: "/securecrt", label: "Bast vs SecureCRT" },
  ],
};
