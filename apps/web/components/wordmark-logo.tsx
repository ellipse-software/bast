import Image from "next/image";

const wordmark = "https://cdn.bast.sh/bast-word-dark.png";

type WordmarkLogoProps = {
  className?: string;
  priority?: boolean;
};

export function WordmarkLogo({
  className = "h-7 w-auto",
  priority = false,
}: WordmarkLogoProps) {
  return (
    <Image
      src={wordmark}
      alt="Bast"
      width={120}
      height={32}
      className={className}
      priority={priority}
    />
  );
}
