import Link from "next/link";

import { WordmarkLogo } from "@/components/wordmark-logo";
import { bastRepoUrl } from "@/lib/github";

const sponsorUrl = "https://github.com/sponsors/tedbrine";

export function SiteHeader() {
  return (
    <header className="relative z-10 w-full">
      <div className="mx-auto flex max-w-5xl items-center justify-between px-4 py-4 sm:px-6">
        <Link href="/" className="shrink-0">
          <WordmarkLogo priority />
        </Link>
        <nav className="flex items-center gap-6 text-sm">
          <Link
            href="/docs"
            className="text-foreground/80 transition-colors hover:text-foreground"
          >
            Docs
          </Link>
          <a
            href={bastRepoUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="text-foreground/80 transition-colors hover:text-foreground"
          >
            GitHub
          </a>
          <a
            href={sponsorUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="text-foreground/80 transition-colors hover:text-foreground"
          >
            Sponsor
          </a>
        </nav>
      </div>
    </header>
  );
}
