import { Code, DocLink } from "@/lib/comparisons/marks";
import type { GuidePage } from "@/lib/guides/types";

export const cloudSshGuide: GuidePage = {
  slug: "cloud-ssh",
  title: "SSH into cloud VMs and sandboxes",
  description:
    "Import live GCP, AWS, Azure, Boat, Upstash Box, Vercel Sandbox, Hetzner Cloud, BitQate, Railway, Fly.io, and DigitalOcean hosts into Bast as read-only inventory. Keep OpenSSH as the connection path for cloud VMs.",
  keywords: [
    "GCP SSH",
    "AWS SSH",
    "Azure SSH",
    "Hetzner SSH",
    "Boat SSH",
    "Boat",
    "BitQate SSH",
    "Railway SSH",
    "Fly.io SSH",
    "DigitalOcean SSH",
    "SSH into EC2",
    "gcloud SSH alternative",
    "cloud VM SSH",
    "Bast.sh",
  ],
  lead: "Cloud consoles and one-off CLI copy-paste are fine until you have dozens of hosts. Bast imports live inventory from GCP, AWS, Azure, Boat, Upstash Box, Vercel Sandbox, Hetzner Cloud, BitQate, Railway, Fly.io, and DigitalOcean, then connects from the picker.",
  problemTitle: "Cloud inventory goes stale the moment you bookmark it",
  problem: [
    <>
      Teams keep EC2 and GCE hosts in notes, SSH config stubs, or GUI session
      lists. Instances get replaced. Private IPs change. Bastion paths evolve.
      The bookmark stays wrong.
    </>,
    <>
      Provider consoles can open a session, but they do not organize the fleet
      the way a terminal-first engineer actually works.
    </>,
  ],
  solutionTitle: "Provider CLIs and APIs in, OpenSSH out",
  solution: [
    <>
      Bast sync pulls hosts through <Code>gcloud</Code>, AWS CLI v2, Azure CLI,
      the Boat <Code>boat</Code> CLI, <Code>flyctl</Code>, and <Code>doctl</Code>,
      or through the Upstash Box, Vercel Sandbox, Hetzner Cloud, BitQate, and
      Railway APIs. Synced hosts are read-only reflections of cloud inventory.
      When sync runs again, the list updates instead of rotting.
    </>,
    <>
      Cloud VMs still use your system <Code>ssh</Code> binary. Vercel Sandboxes
      open a PTY shell instead, because they are not OpenSSH hosts.
    </>,
  ],
  stepsTitle: "Import a cloud account",
  steps: [
    <>
      Authenticate the provider CLI on your machine (<Code>gcloud</Code>,{" "}
      <Code>aws</Code>, <Code>az</Code>, <Code>boat</Code>, <Code>flyctl</Code>,
      or <Code>doctl</Code>), or store a token with <Code>bast upstash key</Code>,{" "}
      <Code>bast vercel token</Code>, <Code>bast hetzner key</Code>,{" "}
      <Code>bast bitqate key</Code>, or <Code>bast railway key</Code>. Fly.io
      and DigitalOcean do not store an API token in Bast.
    </>,
    <>
      Run <Code>bast sync</Code> for <Code>gcp</Code>, <Code>aws</Code>,{" "}
      <Code>azure</Code>, <Code>boat</Code>, <Code>upstash</Code>,{" "}
      <Code>vercel</Code>, <Code>hetzner</Code>, <Code>bitqate</Code>,{" "}
      <Code>railway</Code>, <Code>fly</Code>, or <Code>digitalocean</Code> (or
      use the Sync tab in the TUI). <Code>bast sync do</Code> is an alias of{" "}
      <Code>bast sync digitalocean</Code>.
    </>,
    <>
      Open Bast, find the imported hosts, and connect. Provider notes:{" "}
      <DocLink href="/docs/features/gcp">GCP</DocLink>,{" "}
      <DocLink href="/docs/features/aws">AWS</DocLink>,{" "}
      <DocLink href="/docs/features/azure">Azure</DocLink>,{" "}
      <DocLink href="/docs/features/boat">Boat</DocLink>,{" "}
      <DocLink href="/docs/features/upstash">Upstash Box</DocLink>,{" "}
      <DocLink href="/docs/features/vercel">Vercel Sandbox</DocLink>,{" "}
      <DocLink href="/docs/features/hetzner">Hetzner Cloud</DocLink>,{" "}
      <DocLink href="/docs/features/bitqate">BitQate</DocLink>,{" "}
      <DocLink href="/docs/features/railway">Railway</DocLink>,{" "}
      <DocLink href="/docs/features/fly">Fly.io</DocLink>,{" "}
      <DocLink href="/docs/features/digitalocean">DigitalOcean</DocLink>.
    </>,
  ],
  sections: [
    {
      title: "Keep cloud credentials out of your SSH client",
      paragraphs: [
        <>
          Bast writes local OpenSSH config for imported hosts. GCP, AWS, Azure,
          Boat, Fly.io, and DigitalOcean authenticate through their CLIs. Fly.io
          and DigitalOcean do not store an API token. Upstash Box, Vercel,
          Hetzner Cloud, BitQate, and Railway store tokens in local 0600 files,
          not in SSH config or Vault. Vercel connect is a WebSocket PTY rather
          than OpenSSH.
        </>,
      ],
    },
  ],
};
