"use client";

import { ChevronDown } from "lucide-react";
import Link from "next/link";
import { useEffect, useId, useRef, useState } from "react";

export type NavDropdownItem = {
  href: string;
  label: string;
  blurb?: string;
};

type NavDropdownProps = {
  label: string;
  href: string;
  menuLabel: string;
  toggleLabel: string;
  overview: NavDropdownItem;
  items: readonly NavDropdownItem[];
  align?: "left" | "right";
};

export function NavDropdown({
  label,
  href,
  menuLabel,
  toggleLabel,
  overview,
  items,
  align = "left",
}: NavDropdownProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const menuId = useId();

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

  return (
    <div
      ref={rootRef}
      className="relative"
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
    >
      <div className="flex items-center gap-0.5">
        <Link
          href={href}
          className="text-foreground/80 transition-colors hover:text-foreground"
          onClick={() => setOpen(false)}
        >
          {label}
        </Link>
        <button
          type="button"
          aria-expanded={open}
          aria-controls={menuId}
          aria-label={toggleLabel}
          onClick={() => setOpen((current) => !current)}
          className="inline-flex size-6 items-center justify-center text-foreground/80 transition-colors hover:text-foreground"
        >
          <ChevronDown
            className={`size-3.5 transition-transform ${open ? "rotate-180" : ""}`}
            aria-hidden
          />
        </button>
      </div>

      {open ? (
        <div
          id={menuId}
          role="menu"
          aria-label={menuLabel}
          className={`absolute top-full z-20 pt-2 ${
            align === "right" ? "right-0" : "left-0"
          }`}
        >
          <ul className="min-w-[14rem] border border-border bg-background py-1 shadow-lg">
            <li role="none">
              <Link
                href={overview.href}
                role="menuitem"
                onClick={() => setOpen(false)}
                className="block px-3 py-2 text-sm text-foreground transition-colors hover:bg-surface"
              >
                {overview.label}
                {overview.blurb ? (
                  <span className="mt-0.5 block text-xs text-muted">
                    {overview.blurb}
                  </span>
                ) : null}
              </Link>
            </li>
            <li aria-hidden className="my-1 border-t border-border" />
            {items.map((item) => (
              <li key={item.href} role="none">
                <Link
                  href={item.href}
                  role="menuitem"
                  onClick={() => setOpen(false)}
                  className="block px-3 py-2 text-sm text-foreground/90 transition-colors hover:bg-surface hover:text-foreground"
                >
                  {item.label}
                  {item.blurb ? (
                    <span className="mt-0.5 block text-xs text-muted">
                      {item.blurb}
                    </span>
                  ) : null}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
