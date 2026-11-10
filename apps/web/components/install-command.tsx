"use client";

import { Check, ChevronDown } from "lucide-react";
import {
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";

import { OsMark } from "@/components/os-mark";
import { nightMenuClass } from "@/lib/night";
import { bastReleaseUrl, bastRepoUrl } from "@/lib/github";
import {
  defaultMethodFor,
  getClientInstallPlatform,
  getServerInstallPlatform,
  installCommand,
  installPlatforms,
  methodSupportsNightly,
  methodsForPlatform,
  promptFor,
  resolveDetectedPlatform,
  resolveMethod,
  subscribeInstallPlatform,
  type InstallMethod,
  type InstallPlatform,
} from "@/lib/install";
import { supportsWindowsRelease } from "@/lib/releases";

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
    case "apt":
    case "dnf":
    case "pacman":
    case "apk":
      return (
        <code className="whitespace-nowrap">
          <span className="text-accent">curl</span>
          <span className="text-muted"> -fsSL </span>
          <span className="text-accent">https://packages.bast.sh/setup.sh</span>
          <span className="text-muted"> | </span>
          <span className="text-accent">sudo</span>
          <span className="text-muted"> sh -s -- </span>
          <span className="text-accent">{method}</span>
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
    case "winget":
      return (
        <code className="whitespace-nowrap">
          <span className="text-accent">winget</span>
          <span className="text-muted"> install </span>
          <span className="text-accent">EllipseSoftware.Bast</span>
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

type InstallOption<T extends string> = {
  id: T;
  label: string;
  icon?: ReactNode;
};

type InstallSelectProps<T extends string> = {
  value: T;
  options: InstallOption<T>[];
  open: boolean;
  listboxId: string;
  ariaLabel: string;
  menuAlign?: "start" | "end";
  menuClassName?: string;
  onToggle: () => void;
  onSelect: (id: T) => void;
};

function InstallSelect<T extends string>({
  value,
  options,
  open,
  listboxId,
  ariaLabel,
  menuAlign = "start",
  menuClassName = "bg-background",
  night = false,
  onToggle,
  onSelect,
}: InstallSelectProps<T> & { night?: boolean }) {
  const current = options.find((entry) => entry.id === value);

  return (
    <div className="relative flex">
      <button
        type="button"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listboxId}
        aria-label={`${ariaLabel}: ${current?.label ?? value}`}
        onClick={onToggle}
        className="flex items-center gap-1 whitespace-nowrap px-2 py-0.5 text-[11px] leading-none text-muted transition-colors hover:text-foreground focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-foreground"
      >
        {current?.icon}
        {current?.label ?? value}
        <ChevronDown
          className={`size-3 shrink-0 transition-transform ${open ? "rotate-180" : ""}`}
          aria-hidden
        />
      </button>
      {open ? (
        <ul
          id={listboxId}
          role="listbox"
          aria-label={ariaLabel}
          className={
            night
              ? `night-menu absolute top-full z-20 mt-2 min-w-full whitespace-nowrap py-1.5 ${nightMenuClass} ${
                  menuAlign === "end" ? "right-0" : "left-0"
                }`
              : `absolute top-full z-20 mt-1 min-w-full whitespace-nowrap border border-border py-1 shadow-lg ${menuClassName} ${
                  menuAlign === "end" ? "right-0" : "left-0"
                }`
          }
        >
          {options.map(({ id, label, icon }) => (
            <li key={id} role="option" aria-selected={value === id}>
              <button
                type="button"
                onClick={() => onSelect(id)}
                className={`flex w-full items-center gap-1.5 text-left text-[11px] transition-colors focus-visible:outline focus-visible:outline-1 focus-visible:-outline-offset-1 ${
                  night
                    ? "px-3 py-1.5 hover:bg-white/5 focus-visible:outline-[#c6b6f2]"
                    : "px-2.5 py-1 focus-visible:outline-foreground"
                } ${
                  value === id
                    ? "text-foreground"
                    : "text-muted hover:text-foreground"
                }`}
              >
                {icon}
                {label}
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

type OpenMenu = "platform" | "method" | null;

type InstallCommandProps = {
  version?: string | null;
  wingetAvailable: boolean;
  showVersion?: boolean;
  platformIcons?: boolean;
  controlsPlacement?: "start" | "above";
  className?: string;
};

export function InstallCommand({
  version,
  wingetAvailable,
  showVersion = true,
  platformIcons = false,
  controlsPlacement = "start",
  className = "w-full max-w-xl",
}: InstallCommandProps) {
  const windowsAvailable = supportsWindowsRelease(version);
  const detectedPlatform = useSyncExternalStore(
    subscribeInstallPlatform,
    getClientInstallPlatform,
    getServerInstallPlatform,
  );
  const [selectedPlatform, setSelectedPlatform] =
    useState<InstallPlatform | null>(null);
  const [selectedMethod, setSelectedMethod] = useState<InstallMethod | null>(
    null,
  );
  const [nightly, setNightly] = useState(false);
  const [copied, setCopied] = useState(false);
  const [openMenu, setOpenMenu] = useState<OpenMenu>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const platformListboxId = useId();
  const methodListboxId = useId();
  const platform = resolveDetectedPlatform(
    selectedPlatform ?? detectedPlatform,
    windowsAvailable,
  );
  const method = resolveMethod(
    platform,
    selectedMethod ?? defaultMethodFor(platform),
    wingetAvailable,
  );
  const platforms = installPlatforms(windowsAvailable).map((entry) => ({
    ...entry,
    icon: platformIcons ? (
      <OsMark platform={entry.id} className="size-3 shrink-0" />
    ) : undefined,
  }));
  const methods = methodsForPlatform(platform, wingetAvailable);
  const showNightly = methodSupportsNightly(method);
  const prompt = promptFor(method);
  const controlsAbove = controlsPlacement === "above";

  useEffect(() => {
    if (!openMenu) return;

    function handlePointerDown(event: MouseEvent) {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpenMenu(null);
      }
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpenMenu(null);
      }
    }

    document.addEventListener("mousedown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [openMenu]);

  async function copy() {
    await navigator.clipboard.writeText(installCommand(method, nightly));
    setCopied(true);
    window.setTimeout(() => setCopied(false), 5000);
  }

  function selectPlatform(next: InstallPlatform) {
    setSelectedPlatform(next);
    setCopied(false);
    setOpenMenu(null);
  }

  function selectMethod(next: InstallMethod) {
    setSelectedMethod(next);
    setCopied(false);
    setOpenMenu(null);
  }

  return (
    <div className={className}>
      <div ref={rootRef} className="relative">
        <div
          className={
            controlsAbove
              ? "absolute left-0 top-0 z-10 flex w-max -translate-y-[calc(100%-1px)]"
              : "absolute left-3 top-0 z-10 -translate-y-[calc(50%+6px)]"
          }
        >
          <div
            className={`inline-flex items-stretch border border-border ${
              controlsAbove ? "border-b-0 bg-surface" : "bg-background"
            }`}
          >
            <InstallSelect
              value={platform}
              options={platforms}
              open={openMenu === "platform"}
              listboxId={platformListboxId}
              ariaLabel="Operating system"
              menuClassName={controlsAbove ? "bg-surface" : "bg-background"}
              night={controlsAbove}
              onToggle={() =>
                setOpenMenu((current) =>
                  current === "platform" ? null : "platform",
                )
              }
              onSelect={selectPlatform}
            />
            <div className={controlsAbove ? "contents" : "border-l border-border"}>
              <InstallSelect
                value={method}
                options={methods}
                open={openMenu === "method"}
                listboxId={methodListboxId}
                ariaLabel="Install method"
                menuClassName={controlsAbove ? "bg-surface" : "bg-background"}
                night={controlsAbove}
                onToggle={() =>
                  setOpenMenu((current) =>
                    current === "method" ? null : "method",
                  )
                }
                onSelect={selectMethod}
              />
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
                className={`flex items-center gap-1.5 px-2 py-0.5 text-[11px] leading-none text-muted transition-colors hover:text-foreground ${
                  controlsAbove ? "" : "border-l border-border"
                }`}
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
              className={`flex min-w-0 items-center overflow-x-auto px-4 font-mono text-xs text-foreground sm:flex-1 sm:text-sm ${
                controlsAbove ? "py-3" : "pb-3 pt-4 sm:pb-3 sm:pt-4"
              }`}
              aria-live="polite"
            >
              {copied ? (
                <>
                  <span className="mr-3 shrink-0 text-muted select-none">
                    {prompt}
                  </span>
                  <code className="whitespace-nowrap">
                    <span className="text-accent">bast</span>
                    <span className="text-muted">{" // then try this"}</span>
                  </code>
                </>
              ) : (
                <>
                  <span className="mr-3 shrink-0 text-muted select-none">
                    {prompt}
                  </span>
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
      {showVersion && version ? (
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
