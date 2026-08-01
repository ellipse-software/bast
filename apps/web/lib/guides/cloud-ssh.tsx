import { Code, DocLink } from "@/lib/comparisons/marks";
import type { GuidePage } from "@/lib/guides/types";

export const cloudSshGuide: GuidePage = {
  slug: "cloud-ssh",
  title: "SSH into GCP, AWS, and Azure VMs",
  description:
    "Import live GCP, AWS, and Azure VMs into Bast as read-only SSH hosts. Use each provider CLI, keep OpenSSH as the connection path, and stop maintaining cloud host spreadsheets.",
  keywords: [
    "GCP SSH",
    "AWS SSH",
    "Azure SSH",
    "SSH into EC2",
    "gcloud SSH alternative",
    "cloud VM SSH",
    "Bast.sh",
  ],
  lead: "Cloud consoles and one-off CLI copy-paste are fine until you have dozens of VMs. Bast imports live inventory from GCP, AWS, and Azure, then connects with normal OpenSSH.",
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
      Bast sync pulls VMs through <Code>gcloud</Code>, AWS CLI v2, or Azure CLI.
      Synced hosts are read-only reflections of cloud inventory. When sync runs
      again, the list updates instead of rotting.
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
      <Code>aws</Code>, or <Code>az</Code>).
    </>,
    <>
      Run <Code>bast sync gcp</Code>, <Code>bast sync aws</Code>, or{" "}
      <Code>bast sync azure</Code> (or use the Sync tab in the TUI).
    </>,
    <>
      Open Bast, find the imported hosts, and connect. Start with the{" "}
      <DocLink href="/docs/features/gcp">GCP</DocLink>,{" "}
      <DocLink href="/docs/features/aws">AWS</DocLink>, or{" "}
      <DocLink href="/docs/features/azure">Azure</DocLink> guide for filters and
      networking notes.
    </>,
  ],
  sections: [
    {
      title: "Keep cloud credentials out of your SSH client",
      paragraphs: [
        <>
          Bast does not ask you to paste cloud API keys into a proprietary host
          database. It uses the CLIs you already trust for cloud auth, then
          writes local SSH config for the resulting hosts.
        </>,
      ],
    },
  ],
};
