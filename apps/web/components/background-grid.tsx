type BackgroundGridProps = {
  /** Soft accent wash behind the top of the page (landing hero). */
  accentWash?: boolean;
  /** Module grid behind the page. Off on the landing page. */
  showGrid?: boolean;
};

export function BackgroundGrid({
  accentWash = false,
  showGrid = true,
}: BackgroundGridProps) {
  return (
    <>
      {accentWash ? (
        <div
          aria-hidden
          className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-[min(58vh,40rem)]"
          style={{
            backgroundImage:
              "radial-gradient(ellipse 120% 85% at 50% -20%, color-mix(in srgb, var(--accent) 18%, transparent) 0%, color-mix(in srgb, var(--accent) 8%, transparent) 38%, color-mix(in srgb, var(--accent) 3%, transparent) 62%, transparent 100%)",
          }}
        />
      ) : null}
      {showGrid ? (
        <div
          aria-hidden
          className="pointer-events-none fixed inset-0 -z-10"
          style={{
            backgroundImage: `
          linear-gradient(to right, color-mix(in srgb, var(--color-border) 35%, transparent) 1px, transparent 1px),
          linear-gradient(to bottom, color-mix(in srgb, var(--color-border) 35%, transparent) 1px, transparent 1px)
        `,
            backgroundSize: "48px 48px",
            maskImage:
              "linear-gradient(to bottom, black 0%, rgba(0, 0, 0, 0.55) 28%, rgba(0, 0, 0, 0.2) 55%, transparent 72%)",
            WebkitMaskImage:
              "linear-gradient(to bottom, black 0%, rgba(0, 0, 0, 0.55) 28%, rgba(0, 0, 0, 0.2) 55%, transparent 72%)",
          }}
        />
      ) : null}
    </>
  );
}
