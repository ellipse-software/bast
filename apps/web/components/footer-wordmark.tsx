"use client";

import { useLayoutEffect, useRef } from "react";

/** Full-viewport bast.sh mark: natural tracking, ink gradient, not selectable. */
export function FooterWordmark() {
  const ref = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;

    function fit() {
      if (!el) return;
      const probe = el.querySelector("[data-wordmark-probe]");
      if (!(probe instanceof HTMLElement)) return;

      const target = document.documentElement.clientWidth;
      let low = 8;
      let high = target * 0.55;

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
    window.addEventListener("resize", fit);
    return () => window.removeEventListener("resize", fit);
  }, []);

  return (
    <div
      aria-hidden
      className="pointer-events-none relative left-1/2 mt-4 w-screen max-w-[100vw] -translate-x-1/2 select-none"
    >
      <div ref={ref} className="h-[0.78em] w-full overflow-hidden">
        <p
          data-wordmark-probe
          className="whitespace-nowrap bg-[linear-gradient(to_bottom,rgb(255_255_255/0.055),rgb(255_255_255/0.018))] text-center font-medium leading-none tracking-[-0.045em] text-transparent bg-clip-text [-webkit-background-clip:text]"
          style={{ width: "100vw" }}
        >
          bast.sh
        </p>
      </div>
    </div>
  );
}
