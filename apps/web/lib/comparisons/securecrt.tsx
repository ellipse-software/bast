import { Code, DocLink } from "@/lib/comparisons/marks";
import type { ComparisonCaseStudy } from "@/lib/comparisons/types";

export const securecrtComparison: ComparisonCaseStudy = {
  slug: "securecrt",
  competitorName: "SecureCRT",
  title: "Bast vs SecureCRT",
  description:
    "Bast as a SecureCRT alternative for terminal-first teams: free MIT OpenSSH workflows, ~/.ssh/config instead of proprietary sessions, cloud VM sync, and scriptable JSON CLI.",
  keywords: [
    "Bast vs SecureCRT",
    "SecureCRT alternative",
    "SecureCRT alternative Mac",
    "SecureCRT open source alternative",
    "enterprise SSH client alternative",
    "Bast.sh",
  ],
  lead: "SecureCRT is the long-running commercial SSH/Telnet client enterprises buy when they want support contracts and a polished GUI session manager. Bast is the open-source, terminal-native answer when OpenSSH config is already the company standard.",
  articleHeadline:
    "Bast vs SecureCRT: an open-source terminal alternative to enterprise GUI SSH",
  articleDescription:
    "Why Bast is a better SecureCRT alternative for teams that want native OpenSSH, shared config, and no per-seat client licensing.",
  diffRows: [
    {
      topic: "Model",
      bast: "Open-source terminal TUI + CLI",
      competitor: "Commercial GUI client with paid licenses",
    },
    {
      topic: "SSH stack",
      bast: "System OpenSSH",
      competitor: "SecureCRT's own session engine",
    },
    {
      topic: "Sessions",
      bast: "~/.ssh/config and local Bast metadata",
      competitor: "SecureCRT session hierarchy inside the app",
    },
    {
      topic: "Platforms",
      bast: "macOS, Linux, and Windows 11 terminals",
      competitor: "Desktop apps across major platforms",
    },
    {
      topic: "Cloud hosts",
      bast: "Import via GCP, AWS, Azure, DigitalOcean, and box.ascii.dev CLIs",
      competitor: "Manual or scripted session maintenance",
    },
    {
      topic: "Automation",
      bast: "First-class --json CLI and agent skill",
      competitor: "Scripting exists, but the product is GUI-first",
    },
    {
      topic: "Cost",
      bast: "Free (MIT)",
      competitor: "Per-seat commercial licensing",
    },
  ],
  sections: [
    {
      title: "Enterprise GUI vs engineer terminal",
      paragraphs: [
        <>
          SecureCRT earned its place in NOCs and regulated environments: stable
          GUI sessions, vendor support, and a feature set procurement already
          understands. If your org buys tools that way, SecureCRT is a coherent
          choice.
        </>,
        <>
          Bast is built for engineers who already standardize on OpenSSH. The
          product assumption is that terminals, editors, CI, and infrastructure
          tooling should all read the same host definitions.
        </>,
      ],
    },
    {
      title: "Licensing that does not follow the laptop",
      paragraphs: [
        <>
          SecureCRT is paid software. Seat counts, renewals, and version pinning
          become part of the operational cost, even when the underlying need is
          &quot;browse hosts and SSH quickly.&quot;
        </>,
        <>
          Bast removes that layer. MIT licensing means every engineer can
          install the same host picker without a purchasing ticket for core
          connectivity workflow.
        </>,
      ],
    },
    {
      title: "Sessions that outlive the client",
      paragraphs: [
        <>
          SecureCRT sessions are excellent inside SecureCRT. They are less
          excellent as a shared source of truth for Ansible inventories, remote
          development, and shell aliases.
        </>,
        <>
          Bast treats <Code>~/.ssh/config</Code> as authoritative. That keeps
          human TUI usage and machine automation on one inventory. Encrypted{" "}
          <DocLink href="/docs/features/vault">Vault</DocLink> sync can move
          Bast-managed state between machines without turning host maps into
          opaque vendor data.
        </>,
      ],
    },
    {
      title: "Cloud inventory and agent-ready CLI",
      paragraphs: [
        <>
          Bast imports cloud VMs through provider CLIs and exposes stable JSON
          for scripts and coding agents. That matters when SSH inventory is
          dynamic and partly owned by automation.
        </>,
        <>
          SecureCRT can be scripted, but the center of gravity remains a
          desktop session manager. Bast&apos;s center of gravity is the terminal
          and the OpenSSH files beside it.
        </>,
      ],
    },
  ],
  whenBetterTitle: "When SecureCRT is still the better choice",
  whenBetterIntro: "Keep SecureCRT if:",
  whenBetterItems: [
    "You need a vendor support contract tied to the SSH client itself.",
    "Procurement and training already standardize on SecureCRT across the org.",
    "Operators rely on SecureCRT's GUI session tree as the primary interface.",
    "You require protocols beyond Bast's OpenSSH focus.",
  ],
  whenBetterOutro:
    "Bast is the better SecureCRT alternative when the team already trusts OpenSSH and wants a free, terminal-native host layer instead of another licensed GUI client.",
  migrateTitle: "Switching from SecureCRT to Bast",
  migrateSteps: [
    <>
      Export or recreate SecureCRT sessions as OpenSSH config entries in{" "}
      <Code>~/.ssh/config</Code>.
    </>,
    <>
      Standardize on OpenSSH keys and agents so editors, CI, and Bast all share
      one identity story.
    </>,
    <>
      Install Bast and confirm that labeled hosts connect through system{" "}
      <Code>ssh</Code>.
    </>,
    <>
      Optional: enable <DocLink href="/docs/features/vault">Vault</DocLink> for
      encrypted sync and{" "}
      <DocLink href="/docs/features/cli">CLI JSON</DocLink> for automation.
    </>,
  ],
  faqs: [
    {
      q: "Is Bast a SecureCRT alternative?",
      a: "Yes for teams that want a free, terminal-native OpenSSH host manager instead of a commercial GUI session client.",
    },
    {
      q: "Does Bast include vendor support like SecureCRT?",
      a: "Bast is open source. Support is community and docs oriented, not a commercial support contract bundled with seats.",
    },
    {
      q: "Will Bast replace SecureCRT for every enterprise protocol?",
      a: "No. Bast focuses on OpenSSH workflows on macOS, Linux, and Windows 11. Keep SecureCRT where broader protocol or vendor-support requirements matter.",
    },
    {
      q: "Can SecureCRT and Bast coexist during migration?",
      a: "Yes. Move hosts into ~/.ssh/config first so both tools can target the same inventory while people switch workflows.",
    },
  ],
  related: [
    { href: "/termius", label: "Bast vs Termius" },
    { href: "/putty", label: "Bast vs PuTTY" },
    { href: "/mobaxterm", label: "Bast vs MobaXterm" },
  ],
};
