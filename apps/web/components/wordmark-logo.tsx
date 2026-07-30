import Image from "next/image";

const wordmarkDark = "https://cdn.bast.sh/bast-word-dark.png";
const wordmarkLight = "https://cdn.bast.sh/bast-word.png";

type WordmarkLogoProps = {
  className?: string;
  priority?: boolean;
};

export function WordmarkLogo({
  className = "h-7 w-auto",
  priority = false,
}: WordmarkLogoProps) {
  return (
    <>
      <Image
        src={wordmarkDark}
        alt="Bast"
        width={120}
        height={32}
        className={`${className} hidden dark:block`}
        priority={priority}
      />
      <Image
        src={wordmarkLight}
        alt="Bast"
        width={120}
        height={32}
        className={`${className} block dark:hidden`}
        priority={priority}
      />
    </>
  );
}
