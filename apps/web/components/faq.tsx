"use client";

import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import Link from "next/link";
import { useId, useState, type ReactNode } from "react";

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
        Yes. Bast runs natively on Windows 11 with Windows OpenSSH, including
        Windows Terminal, PowerShell, and Command Prompt. It also runs inside
        WSL as a Linux application. Native Windows and WSL use separate home
        directories and SSH configuration.
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
        text: "Yes. Bast runs natively on Windows 11 with Windows OpenSSH, and it also runs as a Linux application inside WSL.",
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

const panelSpring = {
  type: "spring" as const,
  bounce: 0,
  duration: 0.42,
};

function FaqItem({
  question,
  answer,
}: {
  question: string;
  answer: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const reduceMotion = useReducedMotion();
  const panelId = useId();

  return (
    <div>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((current) => !current)}
        className="flex w-full cursor-pointer items-center justify-between gap-4 px-4 py-4 text-left text-sm font-medium tracking-tight"
      >
        <span>{question}</span>
        <motion.span
          aria-hidden
          className="shrink-0 text-muted"
          animate={{ rotate: open ? 45 : 0 }}
          transition={
            reduceMotion ? { duration: 0 } : { type: "spring", bounce: 0, duration: 0.35 }
          }
        >
          +
        </motion.span>
      </button>

      <AnimatePresence initial={false}>
        {open ? (
          <motion.div
            id={panelId}
            key="panel"
            initial={
              reduceMotion ? { height: "auto" } : { height: 0, opacity: 0.35 }
            }
            animate={{ height: "auto", opacity: 1 }}
            exit={
              reduceMotion
                ? { height: "auto", opacity: 1 }
                : { height: 0, opacity: 0.35 }
            }
            transition={reduceMotion ? { duration: 0 } : panelSpring}
            className="overflow-hidden"
          >
            <div className="px-4 pb-4 text-sm leading-relaxed text-muted">
              {answer}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  );
}

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
            <FaqItem
              key={item.question}
              question={item.question}
              answer={item.answer}
            />
          ))}
        </div>
      </div>
    </section>
  );
}
