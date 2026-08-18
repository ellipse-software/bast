import { Code, DocLink } from "@/lib/comparisons/marks";
import type { GuidePage } from "@/lib/guides/types";

export const syncSshHostsGuide: GuidePage = {
  slug: "sync-ssh-hosts",
  title: "Sync SSH hosts between machines",
  description:
    "Sync SSH hosts and keys between macOS, Linux, and Windows devices with Bast vault. End-to-end encryption on your machine, OpenSSH config stays local, no GUI account lock-in.",
  keywords: [
    "sync SSH hosts",
    "sync SSH config",
    "SSH config sync",
    "encrypted SSH sync",
    "Bast vault",
    "Bast.sh",
  ],
  lead: "Moving between a laptop and a workstation should not mean rebuilding your SSH inventory by hand. Bast vault syncs Bast-managed hosts and keys with encryption that happens on your device.",
  problemTitle: "Copying dotfiles is not really sync",
  problem: [
    <>
      People sync SSH state by dragging <Code>~/.ssh</Code> through chat, git,
      or a password manager note. Private keys get over-shared. Metadata gets
      lost. Two machines drift apart within a week.
    </>,
    <>
      GUI clients solve this with accounts. That works until your host map
      becomes vendor data and paid-plan features gate the workflow.
    </>,
  ],
  solutionTitle: "Bast vault",
  solution: [
    <>
      Vault encrypts Bast-managed hosts, keys, and metadata on your machine
      before anything is uploaded. Bast servers store ciphertext. The passphrase
      never leaves the device.
    </>,
    <>
      Cloud host inventory is separate on purpose: re-sync GCP, AWS, Azure, or
      box.ascii.dev per machine through provider CLIs so cloud credentials stay
      where they belong.
    </>,
  ],
  stepsTitle: "Link two machines",
  steps: [
    <>
      Install Bast on both machines and create or import the hosts and keys you
      want managed.
    </>,
    <>
      Run <Code>bast vault login</Code> and set a passphrase. Link the second
      machine with the same vault.
    </>,
    <>
      Sync, then verify hosts appear and connections still use system{" "}
      <Code>ssh</Code>. Read the <DocLink href="/docs/features/vault">Vault
      docs</DocLink> for exact commands and threat model details.
    </>,
  ],
  sections: [
    {
      title: "What syncs, and what does not",
      paragraphs: [
        <>
          Vault syncs Bast-managed state. It does not try to become the owner of
          every externally maintained SSH config block on your disk. That split
          keeps OpenSSH authoritative while still giving you multi-machine
          continuity.
        </>,
      ],
    },
  ],
};
