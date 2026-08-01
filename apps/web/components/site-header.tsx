"use client";

import { Menu, X } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useId, useState } from "react";

import { ComparisonsNav } from "@/components/comparisons-nav";
import { FeaturesNav } from "@/components/features-nav";
import { WordmarkLogo } from "@/components/wordmark-logo";
import { bastRepoUrl } from "@/lib/github";
import { pageMaxWidthClass } from "@/lib/layout";
import { comparisonNavItems, guideNavItems } from "@/lib/marketing";

const sponsorUrl = "https://github.com/sponsors/tedbrine";

const linkClass =
  "text-foreground/80 transition-colors hover:text-foreground";

const mobileLinkClass =
  "block px-4 py-3 text-sm text-foreground/90 transition-colors hover:bg-surface hover:text-foreground sm:px-6";

const mobileSectionLabelClass =
  "px-4 pb-1 pt-5 text-xs font-medium tracking-tight text-muted first:pt-3 sm:px-6";

export function SiteHeader() {
  const pathname = usePathname();
  const [openForPath, setOpenForPath] = useState<string | null>(null);
  const [scrolled, setScrolled] = useState(false);
  const open = openForPath === pathname;
  const panelId = useId();
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

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpenForPath(null);
      }
    }

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    document.addEventListener("keydown", handleKeyDown);

    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open]);

  return (
    <header
      className={`sticky top-0 z-30 w-full transition-[background-color,border-color,backdrop-filter] duration-200 ease-out motion-reduce:transition-none ${
        frosted
          ? "border-b border-border/70 bg-background/65 backdrop-blur-xl backdrop-saturate-150"
          : "border-b border-transparent bg-transparent"
      }`}
    >
      <div
        className={`mx-auto flex ${pageMaxWidthClass} items-center justify-between gap-4 px-4 py-4 sm:px-6`}
      >
        <Link href="/" className="shrink-0" onClick={() => setOpen(false)}>
          <WordmarkLogo priority />
        </Link>

        <nav
          className="hidden items-center gap-x-6 text-sm md:flex"
          aria-label="Primary"
        >
          <FeaturesNav />
          <ComparisonsNav />
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
            href={sponsorUrl}
            target="_blank"
            rel="noopener noreferrer"
            className={linkClass}
          >
            Sponsor
          </a>
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
        <nav
          id={panelId}
          aria-label="Mobile"
          className="max-h-[calc(100dvh-4.5rem)] overflow-y-auto overscroll-contain border-t border-border bg-background md:hidden"
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
              href={sponsorUrl}
              target="_blank"
              rel="noopener noreferrer"
              className={mobileLinkClass}
              onClick={() => setOpen(false)}
            >
              Sponsor
            </a>
          </div>
        </nav>
      ) : null}
    </header>
  );
}
