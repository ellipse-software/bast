import Image from "next/image";

type DemoScreenshotProps = {
  src: string;
  alt: string;
  width: number;
  height: number;
  label?: string;
};

export function DemoScreenshot({
  src,
  alt,
  width,
  height,
  label,
}: DemoScreenshotProps) {
  return (
    <figure className="not-prose my-6 overflow-hidden rounded-lg border bg-fd-card">
      <Image
        src={src}
        alt={alt}
        width={width}
        height={height}
        className="h-auto w-full"
      />
      <figcaption className="border-t px-4 py-2.5 text-xs text-fd-muted-foreground">
        {label ?? alt}
      </figcaption>
    </figure>
  );
}
