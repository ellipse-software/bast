"use client";

import { Check, ChevronDown } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";

import { bastReleaseUrl, bastRepoUrl } from "@/lib/github";

type InstallMethod =
  | "script"
  | "powershell"
  | "homebrew"
  | "source";

const METHODS: { id: InstallMethod; label: string }[] = [
  { id: "script", label: "Script" },
  { id: "powershell", label: "PowerShell" },
  { id: "homebrew", label: "Homebrew" },
  { id: "source", label: "Source" },
];

function installCommand(method: InstallMethod, nightly: boolean): string {
  switch (method) {
    case "script":
      return nightly
        ? "curl -fsSL https://bast.sh/install-nightly | sh"
        : "curl -fsSL https://bast.sh/install | sh";
    case "homebrew":
      return nightly
        ? "brew install ellipse-software/tap/bast-nightly"
        : "brew install ellipse-software/tap/bast";
    case "powershell":
      return nightly
        ? "irm https://bast.sh/install-nightly.ps1 | iex"
        : "irm https://bast.sh/install.ps1 | iex";
    case "source":
      return `git clone ${bastRepoUrl}.git && cd bast/apps/bast && go build -trimpath -o bast .`;
  }
}

function methodLabel(method: InstallMethod): string {
  return METHODS.find((entry) => entry.id === method)?.label ?? method;
}

function CommandDisplay({
  method,
  nightly,
}: {
  method: InstallMethod;
  nightly: boolean;
}) {
  switch (method) {
    case "script":
      return (
        <code className="whitespace-nowrap">
          <span className="text-accent">curl</span>
          <span className="text-muted"> -fsSL </span>
          <span className="text-accent">
            {nightly
              ? "https://bast.sh/install-nightly"
              : "https://bast.sh/install"}
          </span>
          <span className="text-muted"> | </span>
          <span className="text-accent">sh</span>
        </code>
      );
    case "homebrew":
      return (
        <code className="whitespace-nowrap">
          <span className="text-accent">brew</span>
          <span className="text-muted"> install </span>
          <span className="text-accent">
            {nightly
              ? "ellipse-software/tap/bast-nightly"
              : "ellipse-software/tap/bast"}
          </span>
        </code>
      );
    case "powershell":
      return (
        <code className="whitespace-nowrap">
          <span className="text-accent">irm</span>
          <span className="text-muted"> </span>
          <span className="text-accent">
            {nightly
              ? "https://bast.sh/install-nightly.ps1"
              : "https://bast.sh/install.ps1"}
          </span>
          <span className="text-muted"> | </span>
          <span className="text-accent">iex</span>
        </code>
      );
    case "source":
      return (
        <code className="whitespace-nowrap">
          <span className="text-accent">git clone</span>
          <span className="text-muted"> {bastRepoUrl}.git && </span>
          <span className="text-accent">cd</span>
          <span className="text-muted"> bast/apps/bast && </span>
          <span className="text-accent">go build</span>
          <span className="text-muted"> -trimpath -o bast .</span>
        </code>
      );
  }
}

type InstallCommandProps = {
  version?: string | null;
  className?: string;
};

export function InstallCommand({
  version,
  className = "w-full max-w-xl",
}: InstallCommandProps) {
  const [method, setMethod] = useState<InstallMethod>("script");
  const [nightly, setNightly] = useState(false);
  const [copied, setCopied] = useState(false);
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const listboxId = useId();
  const showNightly =
    method === "script" || method === "powershell" || method === "homebrew";

  useEffect(() => {
    if (!open) return;

    function handlePointerDown(event: MouseEvent) {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
      }
    }

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open]);

  async function copy() {
    await navigator.clipboard.writeText(installCommand(method, nightly));
    setCopied(true);
    window.setTimeout(() => setCopied(false), 5000);
  }

  function selectMethod(next: InstallMethod) {
    setMethod(next);
    setCopied(false);
    setOpen(false);
  }

  return (
    <div className={className}>
      <div ref={rootRef} className="relative">
        <div className="absolute left-3 top-0 z-10 -translate-y-[calc(50%+6px)]">
          <div className="inline-flex items-stretch border border-border bg-background">
            <div className="relative flex">
              <button
                type="button"
                aria-haspopup="listbox"
                aria-expanded={open}
                aria-controls={listboxId}
                onClick={() => setOpen((current) => !current)}
                className="flex items-center gap-1 px-2 py-0.5 text-[11px] leading-none text-muted transition-colors hover:text-foreground"
              >
                {methodLabel(method)}
                <ChevronDown
                  className={`size-3 shrink-0 transition-transform ${open ? "rotate-180" : ""}`}
                  aria-hidden
                />
              </button>
              {open ? (
                <ul
                  id={listboxId}
                  role="listbox"
                  aria-label="Install method"
                  className="absolute left-0 top-full mt-1 min-w-full border border-border bg-background py-1 shadow-lg"
                >
                  {METHODS.map(({ id, label }) => (
                    <li key={id} role="option" aria-selected={method === id}>
                      <button
                        type="button"
                        onClick={() => selectMethod(id)}
                        className={`block w-full px-2.5 py-1 text-left text-[11px] transition-colors ${
                          method === id
                            ? "text-foreground"
                            : "text-muted hover:text-foreground"
                        }`}
                      >
                        {label}
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
            </div>
            {showNightly ? (
              <button
                type="button"
                role="checkbox"
                aria-checked={nightly}
                aria-label="Nightly"
                onClick={() => {
                  setNightly((current) => !current);
                  setCopied(false);
                }}
                className="flex items-center gap-1.5 border-l border-border px-2 py-0.5 text-[11px] leading-none text-muted transition-colors hover:text-foreground"
              >
                <span
                  aria-hidden
                  className={`flex size-2.5 shrink-0 items-center justify-center border ${
                    nightly
                      ? "border-foreground text-foreground"
                      : "border-muted text-transparent"
                  }`}
                >
                  <Check className="size-1.5" strokeWidth={3} />
                </span>
                Nightly
              </button>
            ) : null}
          </div>
        </div>

        <div className="border border-border bg-surface">
          <div className="flex flex-col sm:flex-row sm:items-stretch">
            <div
              className="flex min-w-0 items-center overflow-x-auto px-4 pb-3 pt-4 font-mono text-xs text-foreground sm:flex-1 sm:pb-3 sm:pt-4 sm:text-sm"
              aria-live="polite"
            >
              {copied ? (
                <>
                  <span className="mr-3 shrink-0 text-muted select-none">$</span>
                  <code className="whitespace-nowrap">
                    <span className="text-accent">bast</span>
                    <span className="text-muted">{" // then try this"}</span>
                  </code>
                </>
              ) : (
                <>
                  <span className="mr-3 shrink-0 text-muted select-none">$</span>
                  <CommandDisplay method={method} nightly={nightly} />
                </>
              )}
            </div>
            <button
              type="button"
              onClick={copy}
              className="shrink-0 border-t border-border px-4 py-3 text-xs uppercase tracking-widest text-muted transition-colors hover:bg-background hover:text-foreground sm:border-t-0 sm:border-l"
            >
              {copied ? "Copied" : "Copy"}
            </button>
          </div>
        </div>
      </div>
      {version ? (
        <p className="mt-3 text-center text-xs text-muted">
          Latest release{" "}
          <a
            href={bastReleaseUrl(version)}
            className="font-mono text-foreground/80 transition-colors hover:text-foreground"
          >
            {version}
          </a>
        </p>
      ) : null}
    </div>
  );
}
