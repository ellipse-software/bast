"use client";

import { useLayoutEffect, useRef } from "react";

/** Full-bleed bast.sh mark: natural tracking, ink gradient, not selectable. */
export function FooterWordmark() {
  const ref = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;

    function fit() {
      if (!el) return;
      const probe = el.querySelector("[data-wordmark-probe]");
      if (!(probe instanceof HTMLElement)) return;

      // Use layout width, never 100vw: on Windows classic scrollbars, 100vw is
      // wider than the content box and both overflows the page and collapses
      // this binary search toward the minimum size.
      const target = el.clientWidth;
      if (target <= 0) return;

      let low = 8;
      let high = Math.max(target * 0.55, 9);

      for (let i = 0; i < 24; i += 1) {
        const mid = (low + high) / 2;
        el.style.fontSize = `${mid}px`;
        if (probe.scrollWidth > target) {
          high = mid;
        } else {
          low = mid;
        }
      }

      el.style.fontSize = `${low}px`;
    }

    fit();
    void document.fonts.ready.then(fit);

    const observer = new ResizeObserver(fit);
    observer.observe(el);
    window.addEventListener("resize", fit);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", fit);
    };
  }, []);

  return (
    <div
      aria-hidden
      className="pointer-events-none mt-4 w-full select-none overflow-hidden"
    >
      {/*
        Clip shows the top of the glyphs, then continues through the iOS
        home-indicator inset and the collapsible Safari chrome delta
        (lvh − svh) so the mark meets the toolbar instead of leaving a
        blank pad under the type. Needs viewport-fit=cover.
      */}
      <div
        ref={ref}
        className="h-[0.78em] w-full overflow-hidden"
        style={{
          height:
            "calc(0.78em + env(safe-area-inset-bottom, 0px) + (100lvh - 100svh))",
        }}
      >
        <p
          data-wordmark-probe
          className="mx-auto w-max max-w-none whitespace-nowrap bg-[linear-gradient(to_bottom,rgb(255_255_255/0.055),rgb(255_255_255/0.018))] font-medium leading-none tracking-[-0.045em] text-transparent bg-clip-text [-webkit-background-clip:text]"
        >
          bast.sh
        </p>
      </div>
    </div>
  );
}
