import { Code, DocLink } from "@/lib/comparisons/marks";
import type { GuidePage } from "@/lib/guides/types";

export const cloudSshGuide: GuidePage = {
  slug: "cloud-ssh",
  title: "SSH into GCP, AWS, Azure, DigitalOcean, and Box sandboxes",
  description:
    "Import live GCP, AWS, Azure, and DigitalOcean VMs plus box.ascii.dev and Upstash Box sandboxes into Bast as read-only SSH hosts. Keep OpenSSH as the connection path, and stop maintaining cloud host spreadsheets.",
  keywords: [
    "GCP SSH",
    "AWS SSH",
    "Azure SSH",
    "Box SSH",
    "box.ascii.dev",
    "SSH into EC2",
    "gcloud SSH alternative",
    "DigitalOcean SSH",
    "doctl SSH",
    "cloud VM SSH",
    "Bast.sh",
  ],
  lead: "Cloud consoles and one-off CLI copy-paste are fine until you have dozens of hosts. Bast imports live inventory from GCP, AWS, Azure, DigitalOcean, box.ascii.dev, and Upstash Box, then connects with normal OpenSSH.",
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
  solutionTitle: "Provider CLIs in, OpenSSH out",
  solution: [
    <>
      Bast sync pulls hosts through <Code>gcloud</Code>, AWS CLI v2, Azure CLI,
      doctl, or the ASCII Box <Code>box</Code> CLI. Synced hosts are read-only
      reflections of cloud inventory. When sync runs again, the list updates
      instead of rotting.
    </>,
    <>
      Connecting still uses your system <Code>ssh</Code> binary and the same
      config patterns you already know, including tunnels and identity handling
      documented per provider.
    </>,
  ],
  stepsTitle: "Import a cloud account",
  steps: [
    <>
      Authenticate the provider CLI on your machine (<Code>gcloud</Code>,{" "}
      <Code>aws</Code>, <Code>az</Code>, <Code>doctl</Code>, or <Code>box</Code>), or store an
      Upstash Box API key with <Code>bast upstash key</Code>.
    </>,
    <>
      Run <Code>bast sync gcp</Code>, <Code>bast sync aws</Code>,{" "}
      <Code>bast sync azure</Code>, <Code>bast sync digitalocean</Code>,{" "}
      <Code>bast sync box</Code>, or <Code>bast sync upstash</Code> (or use the Sync tab in the TUI).
    </>,
    <>
      Open Bast, find the imported hosts, and connect. Start with the{" "}
      <DocLink href="/docs/features/gcp">GCP</DocLink>,{" "}
      <DocLink href="/docs/features/aws">AWS</DocLink>,{" "}
      <DocLink href="/docs/features/azure">Azure</DocLink>,{" "}
      <DocLink href="/docs/features/digitalocean">DigitalOcean</DocLink>,{" "}
      <DocLink href="/docs/features/box">box.ascii.dev</DocLink>, or{" "}
      <DocLink href="/docs/features/upstash">Upstash Box</DocLink> guide for
      provider notes.
    </>,
  ],
  sections: [
    {
      title: "Keep cloud credentials out of your SSH client",
      paragraphs: [
        <>
          Bast writes local OpenSSH config for imported hosts. GCP, AWS, Azure,
          DigitalOcean, and box.ascii.dev authenticate through their CLIs. Upstash Box stores
          the API key in a local 0600 file, not in SSH config or Vault.
        </>,
      ],
    },
  ],
};
