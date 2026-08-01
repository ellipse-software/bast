"use client";

import { ChevronDown, Download, ExternalLink, MessageCircle } from "lucide-react";
import { useMemo, useState } from "react";

import {
  installSkillCommand,
  installSkillCurlCommand,
  installSkillProjectCommand,
  llmsFullUrl,
  llmsTxtUrl,
  skillUrl,
} from "@/lib/site";

type AskAiMenuProps = {
  contextUrl: string;
  className?: string;
  label?: string;
};

function buildPrompt(contextUrl: string): string {
  return `Read ${contextUrl} and help me use Bast — the native SSH picker and key manager for macOS/Linux. Bast wraps OpenSSH (not a custom protocol). Prefer \`bast --json\` for automation.`;
}

type AiLink = {
  title: string;
  href: string;
};

export function AskAiMenu({
  contextUrl,
  className = "",
  label = "Ask AI",
}: AskAiMenuProps) {
  const [open, setOpen] = useState(false);
  const prompt = buildPrompt(contextUrl);

  const links = useMemo((): AiLink[] => {
    return [
      {
        title: "Open in ChatGPT",
        href: `https://chatgpt.com/?${new URLSearchParams({ prompt, hints: "search" })}`,
      },
      {
        title: "Open in Claude",
        href: `https://claude.ai/new?${new URLSearchParams({ q: prompt })}`,
      },
      {
        title: "Open in Cursor",
        href: `https://cursor.com/link/prompt?${new URLSearchParams({ text: prompt })}`,
      },
    ];
  }, [prompt]);

  return (
    <div className={`relative inline-flex ${className}`}>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className="inline-flex items-center gap-2 border border-border bg-background px-3 py-1.5 text-xs uppercase tracking-widest text-muted transition-colors hover:bg-surface hover:text-foreground"
      >
        <MessageCircle className="size-3.5" aria-hidden />
        {label}
        <ChevronDown
          className={`size-3.5 transition-transform ${open ? "rotate-180" : ""}`}
          aria-hidden
        />
      </button>
      {open ? (
        <>
          <button
            type="button"
            aria-label="Close menu"
            className="fixed inset-0 z-40 cursor-default"
            onClick={() => setOpen(false)}
          />
          <ul
            role="menu"
            className="absolute right-0 top-full z-50 mt-1 min-w-48 border border-border bg-background py-1 shadow-lg"
          >
            {links.map((link) => (
              <li key={link.href} role="none">
                <a
                  role="menuitem"
                  href={link.href}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={() => setOpen(false)}
                  className="flex items-center gap-2 px-3 py-2 text-sm text-foreground/80 transition-colors hover:bg-surface hover:text-foreground"
                >
                  {link.title}
                  <ExternalLink
                    className="ms-auto size-3.5 text-muted"
                    aria-hidden
                  />
                </a>
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </div>
  );
}

type SkillTarget = {
  title: string;
  command: string;
};

const skillTargets: SkillTarget[] = [
  {
    title: "npx skills (global)",
    command: installSkillCommand,
  },
  {
    title: "npx skills (project)",
    command: installSkillProjectCommand,
  },
  {
    title: "curl (all agents)",
    command: installSkillCurlCommand,
  },
  {
    title: "Cursor only",
    command:
      "mkdir -p ~/.cursor/skills/bast && curl -fsSL https://bast.sh/bast.skill.md -o ~/.cursor/skills/bast/SKILL.md",
  },
  {
    title: "Claude Code only",
    command:
      "mkdir -p ~/.claude/skills/bast && curl -fsSL https://bast.sh/bast.skill.md -o ~/.claude/skills/bast/SKILL.md",
  },
  {
    title: "Codex only",
    command:
      "mkdir -p ~/.codex/skills/bast && curl -fsSL https://bast.sh/bast.skill.md -o ~/.codex/skills/bast/SKILL.md",
  },
];

export function SkillInstallMenu({
  className = "",
  variant = "button",
}: {
  className?: string;
  variant?: "button" | "nav";
}) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);

  async function copyCommand(command: string) {
    await navigator.clipboard.writeText(command);
    setCopied(command);
    window.setTimeout(() => setCopied(null), 2000);
  }

  const triggerClassName =
    variant === "nav"
      ? "inline-flex items-center gap-1 text-sm text-foreground/80 transition-colors hover:text-foreground"
      : "inline-flex items-center gap-2 border border-border bg-background px-3 py-1.5 text-xs uppercase tracking-widest text-muted transition-colors hover:bg-surface hover:text-foreground";

  return (
    <div className={`relative inline-flex ${className}`}>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className={triggerClassName}
      >
        {variant === "button" ? (
          <Download className="size-3.5" aria-hidden />
        ) : null}
        Agent skill
        <ChevronDown
          className={`size-3.5 transition-transform ${open ? "rotate-180" : ""}`}
          aria-hidden
        />
      </button>
      {open ? (
        <>
          <button
            type="button"
            aria-label="Close menu"
            className="fixed inset-0 z-40 cursor-default"
            onClick={() => setOpen(false)}
          />
          <ul
            role="menu"
            className="absolute right-0 top-full z-50 mt-1 w-72 border border-border bg-background py-1 shadow-lg"
          >
            {skillTargets.map((target) => (
              <li key={target.title} role="none">
                <button
                  type="button"
                  role="menuitem"
                  onClick={() => copyCommand(target.command)}
                  className="block w-full px-3 py-2 text-left text-sm text-foreground/80 transition-colors hover:bg-surface hover:text-foreground"
                >
                  {copied === target.command ? "Copied!" : `Copy install · ${target.title}`}
                </button>
              </li>
            ))}
            <li role="none" className="border-t border-border">
              <a
                role="menuitem"
                href={skillUrl}
                download="bast.skill.md"
                onClick={() => setOpen(false)}
                className="flex items-center gap-2 px-3 py-2 text-sm text-foreground/80 transition-colors hover:bg-surface hover:text-foreground"
              >
                Download SKILL.md
                <ExternalLink
                  className="ms-auto size-3.5 text-muted"
                  aria-hidden
                />
              </a>
            </li>
          </ul>
        </>
      ) : null}
    </div>
  );
}

export function AgentResources({
  contextUrl = llmsTxtUrl,
}: {
  contextUrl?: string;
}) {
  return (
    <div className="flex flex-wrap items-center justify-center gap-3">
      <AskAiMenu contextUrl={contextUrl} />
      <SkillInstallMenu />
      <a
        href={llmsFullUrl}
        className="inline-flex items-center gap-2 border border-border bg-background px-3 py-1.5 text-xs uppercase tracking-widest text-muted transition-colors hover:bg-surface hover:text-foreground"
      >
        llms-full.txt
      </a>
    </div>
  );
}
