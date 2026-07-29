export function BackgroundGrid() {
  return (
    <div
      aria-hidden
      className="pointer-events-none fixed inset-0 -z-10"
      style={{
        backgroundImage: `
          linear-gradient(to right, rgba(255, 255, 255, 0.04) 1px, transparent 1px),
          linear-gradient(to bottom, rgba(255, 255, 255, 0.04) 1px, transparent 1px)
        `,
        backgroundSize: "48px 48px",
        maskImage:
          "linear-gradient(to bottom, black 0%, rgba(0, 0, 0, 0.6) 35%, transparent 85%)",
        WebkitMaskImage:
          "linear-gradient(to bottom, black 0%, rgba(0, 0, 0, 0.6) 35%, transparent 85%)",
      }}
    />
  );
}
