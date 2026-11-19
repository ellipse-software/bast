"use client";

import { Menu, X } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useId, useRef, useState } from "react";

import { ComparisonsNav } from "@/components/comparisons-nav";
import { FeaturesNav } from "@/components/features-nav";
import { SponsorNavButton } from "@/components/sponsor-dialog-context";
import { WordmarkLogo } from "@/components/wordmark-logo";
import { bastRepoUrl } from "@/lib/github";
import { discordUrl } from "@/lib/site";
import { pageMaxWidthClass } from "@/lib/layout";
import { comparisonNavItems, guideNavItems } from "@/lib/marketing";
import { nightTheme } from "@/lib/night";

const linkClass =
  "text-foreground/80 transition-colors hover:text-foreground";

const mobileLinkClass =
  "block px-4 py-3 text-sm text-foreground/90 transition-colors hover:bg-surface hover:text-foreground sm:px-6";

const mobileSectionLabelClass =
  "px-4 pb-1 pt-5 text-xs font-medium tracking-tight text-muted first:pt-3 sm:px-6";

const focusableSelector = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(", ");

function visibleFocusables(root: HTMLElement): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>(focusableSelector)].filter(
    (element) => element.getClientRects().length > 0,
  );
}

/** Mark everything outside `keep` inert, and return a restore function. */
function inertOutside(keep: HTMLElement): () => void {
  const changed: HTMLElement[] = [];
  let current = keep;
  let parent = current.parentElement;
  while (parent && parent !== document.documentElement) {
    for (const child of parent.children) {
      if (child === current || !(child instanceof HTMLElement) || child.inert) {
        continue;
      }
      child.inert = true;
      changed.push(child);
    }
    current = parent;
    parent = current.parentElement;
  }
  return () => {
    for (const element of changed) element.inert = false;
  };
}

export function SiteHeader({
  wide = false,
  tone = "night",
}: {
  wide?: boolean;
  tone?: "default" | "night";
}) {
  const night = tone === "night";
  const pathname = usePathname();
  const [openForPath, setOpenForPath] = useState<string | null>(null);
  const [scrolled, setScrolled] = useState(false);
  const open = openForPath === pathname;
  const panelId = useId();
  const headerRef = useRef<HTMLElement>(null);
  const frosted = scrolled || open;

  function setOpen(next: boolean) {
    setOpenForPath(next ? pathname : null);
  }

  useEffect(() => {
    function updateScrolled() {
      const next = window.scrollY > 8;
      setScrolled((current) => (current === next ? current : next));
    }

    updateScrolled();
    window.addEventListener("scroll", updateScrolled, { passive: true });
    return () => window.removeEventListener("scroll", updateScrolled);
  }, []);

  useEffect(() => {
    if (!open) return;
    const header = headerRef.current;
    if (!header) return;

    const mdBreakpoint =
      getComputedStyle(document.documentElement)
        .getPropertyValue("--breakpoint-md")
        .trim() || "48rem";
    const desktop = window.matchMedia(`(min-width: ${mdBreakpoint})`);

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpenForPath(null);
        return;
      }
      if (event.key !== "Tab" || !header) return;

      const items = visibleFocusables(header);
      if (items.length === 0) return;
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;

      if (!(active instanceof HTMLElement) || !header.contains(active)) {
        event.preventDefault();
        (event.shiftKey ? last : first).focus();
        return;
      }
      if (event.shiftKey && active === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && active === last) {
        event.preventDefault();
        first.focus();
      }
    }

    const closeOnDesktop = () => {
      if (desktop.matches) setOpenForPath(null);
    };
    desktop.addEventListener("change", closeOnDesktop);
    if (desktop.matches) {
      return () => desktop.removeEventListener("change", closeOnDesktop);
    }

    const releaseInert = inertOutside(header);
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", handleKeyDown);

    return () => {
      releaseInert();
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", handleKeyDown);
      desktop.removeEventListener("change", closeOnDesktop);
    };
  }, [open]);

  return (
    <header
      ref={headerRef}
      style={night ? nightTheme : undefined}
      className={`sticky top-0 z-30 w-full transition-[background-color,border-color,backdrop-filter] duration-200 ease-out motion-reduce:transition-none ${
        frosted
          ? "border-b border-border/70 bg-background/65 backdrop-blur-xl backdrop-saturate-150"
          : "border-b border-transparent bg-transparent"
      }`}
    >
      <div
        className={`mx-auto flex items-center justify-between gap-4 px-4 py-4 sm:px-6 ${
          wide ? "w-full lg:px-10" : pageMaxWidthClass
        }`}
      >
        <Link href="/" className="shrink-0" onClick={() => setOpen(false)}>
          <WordmarkLogo priority color="white" />
        </Link>

        <nav
          className="hidden items-center gap-x-6 text-sm md:flex"
          aria-label="Primary"
        >
          <FeaturesNav tone={tone} />
          <ComparisonsNav tone={tone} />
          <Link href="/docs" className={linkClass}>
            Docs
          </Link>
          <Link href="/changelog" className={linkClass}>
            Changelog
          </Link>
          <a
            href={bastRepoUrl}
            target="_blank"
            rel="noopener noreferrer"
            className={linkClass}
          >
            GitHub
          </a>
          <a
            href={discordUrl}
            target="_blank"
            rel="noopener noreferrer"
            className={linkClass}
          >
            Discord
          </a>
          <SponsorNavButton className={linkClass} />
        </nav>

        <button
          type="button"
          className={`inline-flex size-10 items-center justify-center text-foreground transition-colors hover:bg-surface md:hidden ${
            frosted ? "border border-border" : "border border-transparent"
          }`}
          aria-expanded={open}
          aria-controls={panelId}
          aria-label={open ? "Close menu" : "Open menu"}
          onClick={() => setOpen(!open)}
        >
          {open ? (
            <X className="size-5" strokeWidth={1.75} aria-hidden />
          ) : (
            <Menu className="size-5" strokeWidth={1.75} aria-hidden />
          )}
        </button>
      </div>

      {open ? (
        // Out of flow: an in-flow panel grows the hero, and the dither recaptures.
        // svh keeps this cap inside the small viewport and stops it resizing with browser chrome.
        <nav
          id={panelId}
          aria-label="Mobile"
          className="max-h-svh-header absolute inset-x-0 top-full z-40 overflow-y-auto overscroll-contain border-t border-border bg-background md:hidden"
        >
          <div className={`mx-auto ${pageMaxWidthClass} pb-8`}>
            <p className={mobileSectionLabelClass}>Features</p>
            <Link
              href="/features"
              className={mobileLinkClass}
              onClick={() => setOpen(false)}
            >
              All features
            </Link>
            {guideNavItems.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className={mobileLinkClass}
                onClick={() => setOpen(false)}
              >
                <span className="block">{item.label}</span>
                <span className="mt-0.5 block text-xs text-muted">
                  {item.blurb}
                </span>
              </Link>
            ))}

            <p className={mobileSectionLabelClass}>Comparisons</p>
            <Link
              href="/alternatives"
              className={mobileLinkClass}
              onClick={() => setOpen(false)}
            >
              All comparisons
            </Link>
            {comparisonNavItems.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className={mobileLinkClass}
                onClick={() => setOpen(false)}
              >
                <span className="block">vs {item.label}</span>
                <span className="mt-0.5 block text-xs text-muted">
                  {item.blurb}
                </span>
              </Link>
            ))}

            <p className={mobileSectionLabelClass}>Product</p>
            <Link
              href="/docs"
              className={mobileLinkClass}
              onClick={() => setOpen(false)}
            >
              Docs
            </Link>
            <Link
              href="/changelog"
              className={mobileLinkClass}
              onClick={() => setOpen(false)}
            >
              Changelog
            </Link>
            <a
              href={bastRepoUrl}
              target="_blank"
              rel="noopener noreferrer"
              className={mobileLinkClass}
              onClick={() => setOpen(false)}
            >
              GitHub
            </a>
            <a
              href={discordUrl}
              target="_blank"
              rel="noopener noreferrer"
              className={mobileLinkClass}
              onClick={() => setOpen(false)}
            >
              Discord
            </a>
            <SponsorNavButton
              className={`${mobileLinkClass} w-full text-left`}
              onClick={() => setOpen(false)}
            />
          </div>
        </nav>
      ) : null}
    </header>
  );
}
