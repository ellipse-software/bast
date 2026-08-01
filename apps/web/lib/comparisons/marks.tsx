import Link from "next/link";
import type { ReactNode } from "react";

export function Code({ children }: { children: ReactNode }) {
  return <code className="text-foreground">{children}</code>;
}

export function DocLink({
  href,
  children,
}: {
  href: string;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className="text-foreground underline-offset-2 hover:underline"
    >
      {children}
    </Link>
  );
}
