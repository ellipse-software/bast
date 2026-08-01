import { Code, DocLink } from "@/lib/comparisons/marks";
import type { GuidePage } from "@/lib/guides/types";

export const sshKeyManagerGuide: GuidePage = {
  slug: "ssh-key-manager",
  title: "SSH key manager that stays on disk",
  description:
    "Generate, import, export, and install OpenSSH keys with Bast. Keys stay as normal files on your machine. No proprietary key vault required for day-to-day SSH.",
  keywords: [
    "SSH key manager",
    "manage SSH keys",
    "generate SSH key",
    "OpenSSH key manager",
    "Bast.sh",
  ],
  lead: "SSH keys should be ordinary files your agent, CI, and editors understand. Bast is an SSH key manager for generating and organizing those files without a proprietary lock-in store.",
  problemTitle: "Keys trapped in an app are keys you fight later",
  problem: [
    <>
      GUI clients often manage identities inside their own world. Exporting,
      rotating, or reusing those keys with OpenSSH becomes a conversion chore.
    </>,
    <>
      The other extreme is a pile of unnamed files in <Code>~/.ssh</Code> with
      no sense of which host uses which identity.
    </>,
  ],
  solutionTitle: "Native keys, clearer workflow",
  solution: [
    <>
      Bast generates, imports, exports, and installs standard OpenSSH keys.
      Passphrases stay local. Files live under predictable paths such as{" "}
      <Code>~/.ssh/bast/keys/</Code>.
    </>,
    <>
      When you attach an identity to a host, that choice is still expressed in
      SSH config terms other tools can read.
    </>,
  ],
  stepsTitle: "Create and use a key",
  steps: [
    <>
      Run <Code>bast keys generate work</Code> or use the Keys tab in the TUI.
    </>,
    <>
      Install the public key on a host with{" "}
      <Code>bast keys install work --host &lt;host&gt;</Code>, or point the host
      at the identity file in config.
    </>,
    <>
      Use <DocLink href="/docs/features/keys">Keys</DocLink> and{" "}
      <DocLink href="/docs/features/key-selector">Key selector</DocLink> docs
      when you need import/export details.
    </>,
  ],
  sections: [
    {
      title: "Optional encrypted sync",
      paragraphs: [
        <>
          If you need the same Bast-managed keys on another machine,{" "}
          <DocLink href="/docs/features/vault">Vault</DocLink> can sync that
          state end-to-end encrypted. Day-to-day SSH still speaks normal key
          files.
        </>,
      ],
    },
  ],
};
