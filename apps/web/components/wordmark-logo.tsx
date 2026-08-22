import Image from "next/image";

type WordmarkLogoProps = {
  className?: string;
  priority?: boolean;
  /** `white` for the site header; default keeps the SVG accent fill. */
  color?: "accent" | "white";
};

export function WordmarkLogo({
  className = "h-7 w-auto",
  priority = false,
  color = "accent",
}: WordmarkLogoProps) {
  return (
    <Image
      src="/bast-word.svg"
      alt="Bast"
      width={472}
      height={140}
      unoptimized
      className={`${className}${color === "white" ? " brightness-0 invert" : ""}`}
      priority={priority}
    />
  );
}
