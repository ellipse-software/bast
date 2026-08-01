import { Code, DocLink } from "@/lib/comparisons/marks";
import type { ComparisonCaseStudy } from "@/lib/comparisons/types";

export const termiusComparison: ComparisonCaseStudy = {
  slug: "termius",
  competitorName: "Termius",
  title: "Bast vs Termius",
  description:
    "A deep dive into Bast as a Termius alternative: terminal-native OpenSSH, hosts in ~/.ssh/config, free MIT licensing, cloud VM sync, and encrypted vault sync without account lock-in.",
  keywords: [
    "Bast vs Termius",
    "Termius alternative",
    "Termius alternative open source",
    "SSH client terminal",
    "OpenSSH host manager",
    "Bast.sh",
  ],
  lead: "Termius is a polished GUI SSH client. Bast is a terminal-native host picker that refuses to own your config. If you already live in OpenSSH, Bast is usually the better Termius alternative.",
  articleHeadline:
    "Bast vs Termius: a terminal-native alternative to GUI SSH clients",
  articleDescription:
    "Why Bast is a better fit than Termius for developers who want OpenSSH, ~/.ssh/config, and a free terminal host picker.",
  diffRows: [
    {
      topic: "Interface",
      bast: "Runs inside the terminal you already use",
      competitor: "Separate GUI app on desktop and mobile",
    },
    {
      topic: "SSH stack",
      bast: "Launches your system OpenSSH binary",
      competitor: "Uses Termius's own SSH client",
    },
    {
      topic: "Host storage",
      bast: "Native ~/.ssh/config plus one managed Include",
      competitor: "Proprietary host database inside the app",
    },
    {
      topic: "Keys",
      bast: "Standard OpenSSH keys on disk",
      competitor: "Managed inside Termius, with plan limits on extras",
    },
    {
      topic: "Cloud VMs",
      bast: "Import from GCP, AWS, and Azure via their CLIs",
      competitor: "Mostly manual host setup",
    },
    {
      topic: "Machine sync",
      bast: "Optional end-to-end encrypted Bast vault",
      competitor: "Account-based sync on paid plans",
    },
    {
      topic: "Automation",
      bast: "First-class CLI with stable --json output",
      competitor: "Built for interactive GUI workflows",
    },
    {
      topic: "Price",
      bast: "Free and open source (MIT)",
      competitor: "Free tier; sync, SFTP extras, and teams are paid",
    },
  ],
  sections: [
    {
      title: "The actual product difference",
      paragraphs: [
        <>
          Most &quot;SSH client&quot; comparisons flatten into feature
          checklists. The real split is simpler: Termius wants to be your SSH
          home. Bast wants OpenSSH to stay your SSH home.
        </>,
        <>
          Termius ships a dedicated app, its own connection engine, and a host
          database that lives inside that product. That is a good design if you
          want one branded workspace across phones and laptops, and you are
          willing to keep inventory there.
        </>,
        <>
          Bast runs in the terminal, reads the <Code>~/.ssh/config</Code> you
          already have, and launches the <Code>ssh</Code> binary already on your
          PATH. Groups, tags, colors, notes, and favorites sit in local
          metadata. Connection facts stay in OpenSSH files where every other
          tool can see them.
        </>,
      ],
    },
    {
      title: "Why host lock-in matters",
      paragraphs: [
        <>
          Once hosts live only inside a GUI client, your shell scripts, Ansible
          inventories, VS Code Remote, Cursor, CI jump hosts, and teammates&apos;
          configs stop sharing one source of truth. You end up maintaining two
          worlds: the app database and the real SSH config.
        </>,
        <>
          Bast refuses that split. On first run it adds a single{" "}
          <Code>Include</Code> for managed hosts. Everything else stays where
          you left it. Delete Bast tomorrow and <Code>ssh</Code> still works the
          same way.
        </>,
        <>
          That is the core reason Bast beats Termius for terminal-first
          engineers: the product improves discovery and organization without
          becoming the system of record.
        </>,
      ],
    },
    {
      title: "Native OpenSSH, not a parallel client",
      paragraphs: [
        <>
          Termius implements its own SSH stack. That unlocks a consistent
          experience across platforms, but it also means ProxyJump quirks,
          IdentityAgent setups, Match blocks, Include trees, and local SSH
          config features have to be re-expressed inside Termius&apos;s model.
        </>,
        <>
          Bast does not reimplement SSH. It prepares the host, then hands off to
          system OpenSSH. If <Code>ssh -G</Code> understands your config, Bast
          does too. File transfers use the same config through SFTP in the TUI,
          including ProxyJump and cloud ProxyCommand tunnels.
        </>,
      ],
    },
    {
      title: "Cloud VMs without spreadsheet busywork",
      paragraphs: [
        <>
          Bast imports live inventory from GCP, AWS, and Azure through each
          provider&apos;s CLI. Synced hosts stay read-only and owned by the
          cloud. When an instance disappears upstream, sync removes it locally
          instead of leaving stale GUI bookmarks behind.
        </>,
        <>
          Termius can store whatever hosts you type in. Bast treats cloud fleets
          as something to discover, not something to re-key by hand every
          sprint.
        </>,
      ],
    },
    {
      title: "Sync without turning your hosts into SaaS data",
      paragraphs: [
        <>
          Termius sync is account-shaped: create an account, pay for the plan
          that unlocks the extras you need, keep inventory in their product.
        </>,
        <>
          Bast vault encrypts Bast-managed hosts, keys, and metadata on your
          machine before anything is uploaded. Servers only store ciphertext.
          The passphrase never leaves the device. Cloud VM inventory still
          re-syncs per machine through provider CLIs, which keeps credentials
          where they belong.
        </>,
        <>
          If you want encrypted multi-machine continuity without making a GUI
          vendor the owner of your fleet map, Bast is the clearer fit.
        </>,
      ],
    },
    {
      title: "Price, automation, and AI agents",
      paragraphs: [
        <>
          Bast is free and open source under MIT. There is no paid gate in front
          of SFTP, sync of Bast-managed state, cloud import, or the host picker.
          Termius&apos;s free tier is real, but the features teams usually want
          (broader sync, richer SFTP, collaboration) sit on paid plans.
        </>,
        <>
          Bast also ships a CLI meant for scripts and agents:{" "}
          <Code>bast hosts list --json</Code>, structured errors, and an
          installable agent skill. Termius is optimized for humans clicking
          through a GUI. Bast is optimized for humans in a TUI and machines that
          speak JSON.
        </>,
      ],
    },
  ],
  whenBetterTitle: "When Termius is still the better choice",
  whenBetterIntro:
    "Bast is not a universal replacement. Keep Termius if any of these are the job:",
  whenBetterItems: [
    "You need a polished native iOS or Android SSH app as the primary client.",
    "Your team wants one GUI workspace and is fine storing hosts inside that product.",
    "You are on Windows today. Bast targets macOS and Linux terminals; Windows support is not there yet.",
    "You prefer a full graphical terminal emulator bundled with the client rather than using the shell you already have.",
  ],
  whenBetterOutro:
    "Honest positioning beats fake checkmate comparisons. Bast wins for terminal-native OpenSSH workflows. Termius wins for native mobile GUI SSH and account-centric fleets.",
  migrateTitle: "Switching from Termius to Bast",
  migrateSteps: [
    <>
      Recreate important hosts as OpenSSH config entries, or export whatever
      Termius can give you into <Code>~/.ssh/config</Code>.
    </>,
    <>
      Install Bast with the command below. On first run it discovers existing
      hosts and adds one managed Include.
    </>,
    <>
      Organize with groups, tags, colors, and notes inside Bast. Leave
      hostnames, users, ports, and identities in SSH config.
    </>,
    <>
      Optionally enable <DocLink href="/docs/features/vault">Vault</DocLink> for
      encrypted sync of Bast-managed state, and{" "}
      <DocLink href="/docs/features/gcp">cloud sync</DocLink> for live VMs.
    </>,
  ],
  faqs: [
    {
      q: "Is Bast a Termius alternative?",
      a: "Yes, for terminal-first workflows. Bast keeps hosts in OpenSSH config, launches system ssh, and stays MIT-licensed.",
    },
    {
      q: "Why choose Bast over Termius?",
      a: "Faster host picking without leaving the terminal, no proprietary host database, cloud VM import, encrypted vault sync, and no paid gate on core features.",
    },
    {
      q: "Does Bast replace Termius on phones?",
      a: "No as a native app. Bast works in narrow SSH terminals, including phone SSH apps, but Termius remains stronger as a native mobile client.",
    },
    {
      q: "Can I migrate from Termius to Bast?",
      a: "Yes. Put hosts back into ~/.ssh/config, install Bast, and keep using the same keys and ssh binary.",
    },
  ],
  related: [
    { href: "/putty", label: "Bast vs PuTTY" },
    { href: "/mobaxterm", label: "Bast vs MobaXterm" },
    { href: "/securecrt", label: "Bast vs SecureCRT" },
  ],
};
