import type { ReactNode } from "react";
import Link from "next/link";

const faqs: { question: string; answer: ReactNode }[] = [
  {
    question: "Does Bast replace OpenSSH?",
    answer: (
      <>
        No. Bast launches the <code className="text-foreground">ssh</code> binary
        already on your machine and reads your existing{" "}
        <code className="text-foreground">~/.ssh/config</code>. It adds a managed{" "}
        <code className="text-foreground">Include</code> for hosts you create in
        Bast. Your config stays the source of truth. See{" "}
        <Link
          href="/docs/features/ssh-config"
          className="text-foreground underline-offset-2 hover:underline"
        >
          SSH config
        </Link>
        .
      </>
    ),
  },
  {
    question: "Does Bast work on Windows?",
    answer: (
      <>
        Not yet. Bast targets macOS and Linux terminals. On Windows, keep using
        OpenSSH, Windows Terminal, or a classic client like PuTTY until native
        support exists.
      </>
    ),
  },
  {
    question: "Where are hosts and keys stored?",
    answer: (
      <>
        Connection settings stay in OpenSSH config. Bast-managed hosts live under{" "}
        <code className="text-foreground">~/.ssh/bast/</code>, keys under{" "}
        <code className="text-foreground">~/.ssh/bast/keys/</code>, and metadata
        (groups, tags, notes) in{" "}
        <code className="text-foreground">~/.config/bast/state.json</code>. Full
        layout:{" "}
        <Link
          href="/docs/reference/files"
          className="text-foreground underline-offset-2 hover:underline"
        >
          Files and storage
        </Link>
        .
      </>
    ),
  },
  {
    question: "Is Bast vault end-to-end encrypted?",
    answer: (
      <>
        Yes. Vault encrypts Bast-managed hosts, keys, and metadata on your
        machine before anything is uploaded. Bast servers only store ciphertext.
        The passphrase never leaves your device. Details:{" "}
        <Link
          href="/docs/features/vault"
          className="text-foreground underline-offset-2 hover:underline"
        >
          Vault
        </Link>
        .
      </>
    ),
  },
  {
    question: "How is Bast different from Termius?",
    answer: (
      <>
        Termius is a GUI client with its own host database and account sync.
        Bast stays in the terminal, speaks native OpenSSH, and keeps hosts in{" "}
        <code className="text-foreground">~/.ssh/config</code>. Pick Bast when
        you want a faster picker without leaving your SSH setup behind. Full
        write-up:{" "}
        <Link
          href="/termius"
          className="text-foreground underline-offset-2 hover:underline"
        >
          Bast vs Termius
        </Link>
        .
      </>
    ),
  },
  {
    question: "Can I automate Bast or use it with AI agents?",
    answer: (
      <>
        Yes. Prefer <code className="text-foreground">bast --json</code> for
        scripts, and install the agent skill with{" "}
        <code className="text-foreground">
          npx skills add ellipse-software/bast -g -y
        </code>
        . See{" "}
        <Link
          href="/docs/features/cli"
          className="text-foreground underline-offset-2 hover:underline"
        >
          CLI
        </Link>{" "}
        and{" "}
        <Link
          href="/docs/reference/agents"
          className="text-foreground underline-offset-2 hover:underline"
        >
          AI agents
        </Link>
        .
      </>
    ),
  },
];

const faqJsonLd = {
  "@context": "https://schema.org",
  "@type": "FAQPage",
  mainEntity: [
    {
      "@type": "Question",
      name: "Does Bast replace OpenSSH?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "No. Bast launches the ssh binary already on your machine and reads your existing ~/.ssh/config. It adds a managed Include for hosts you create in Bast. Your config stays the source of truth.",
      },
    },
    {
      "@type": "Question",
      name: "Does Bast work on Windows?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "Not yet. Bast targets macOS and Linux terminals.",
      },
    },
    {
      "@type": "Question",
      name: "Where are hosts and keys stored?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "Connection settings stay in OpenSSH config. Bast-managed hosts live under ~/.ssh/bast/, keys under ~/.ssh/bast/keys/, and metadata in ~/.config/bast/state.json.",
      },
    },
    {
      "@type": "Question",
      name: "Is Bast vault end-to-end encrypted?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "Yes. Vault encrypts Bast-managed hosts, keys, and metadata on your machine before upload. Bast servers only store ciphertext. The passphrase never leaves your device.",
      },
    },
    {
      "@type": "Question",
      name: "How is Bast different from Termius?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "Termius is a GUI client with its own host database and account sync. Bast stays in the terminal, speaks native OpenSSH, and keeps hosts in ~/.ssh/config.",
      },
    },
    {
      "@type": "Question",
      name: "Can I automate Bast or use it with AI agents?",
      acceptedAnswer: {
        "@type": "Answer",
        text: "Yes. Prefer bast --json for scripts, and install the agent skill with npx skills add ellipse-software/bast -g -y.",
      },
    },
  ],
};

export function Faq() {
  return (
    <section className="w-full">
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(faqJsonLd) }}
      />
      <div className="mb-6 max-w-xl">
        <h2 className="mb-2 text-lg font-medium tracking-tight">FAQ</h2>
        <p className="text-sm leading-relaxed text-muted">
          Short answers to the questions people ask before installing.
        </p>
      </div>

      <div className="bg-border p-px">
        <div className="divide-y divide-border bg-background">
          {faqs.map((item) => (
            <details key={item.question} className="group">
              <summary className="flex cursor-pointer list-none items-center justify-between gap-4 px-4 py-4 text-sm font-medium tracking-tight marker:content-none [&::-webkit-details-marker]:hidden">
                <span>{item.question}</span>
                <span
                  aria-hidden
                  className="shrink-0 text-muted transition-transform group-open:rotate-45"
                >
                  +
                </span>
              </summary>
              <div className="px-4 pb-4 text-sm leading-relaxed text-muted">
                {item.answer}
              </div>
            </details>
          ))}
        </div>
      </div>
    </section>
  );
}
