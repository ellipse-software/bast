import Link from "next/link";

import {
  MarketingBreadcrumb,
  MarketingShell,
} from "@/components/marketing-shell";
import {
  bastReleasesUrl,
  getBastReleases,
  getLatestBastVersion,
} from "@/lib/github";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = createPageMetadata({
  title: "Changelog",
  description:
    "Bast release history. Stable versions pulled from GitHub Releases, with links back to full notes and downloads.",
  path: "/changelog",
});

function formatDate(value: string | null): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat("en-GB", {
    year: "numeric",
    month: "short",
    day: "numeric",
  }).format(date);
}

function ReleaseBody({ body }: { body: string }) {
  if (!body) {
    return (
      <p className="text-sm text-muted">No release notes for this tag.</p>
    );
  }

  const lines = body.split(/\r?\n/);

  return (
    <div className="space-y-2 text-sm leading-relaxed text-muted">
      {lines.map((line, index) => {
        const trimmed = line.trim();
        if (!trimmed) return <div key={`blank-${index}`} className="h-2" />;

        const bullet = trimmed.match(/^[-*]\s+(.*)$/);
        const content = bullet ? bullet[1] : trimmed.replace(/^#+\s*/, "");

        const nodes = linkify(content);

        if (bullet) {
          return (
            <p key={index} className="pl-4">
              <span className="mr-2 text-foreground/50">-</span>
              {nodes}
            </p>
          );
        }

        if (/^#+\s/.test(trimmed) || /^\*\*.*\*\*$/.test(trimmed)) {
          return (
            <p key={index} className="font-medium text-foreground">
              {nodes}
            </p>
          );
        }

        return <p key={index}>{nodes}</p>;
      })}
    </div>
  );
}

function linkify(text: string) {
  const parts = text.split(/(https?:\/\/[^\s)]+|#[0-9]+)/g);
  return parts.map((part, index) => {
    if (/^https?:\/\//.test(part)) {
      return (
        <a
          key={index}
          href={part}
          target="_blank"
          rel="noopener noreferrer"
          className="text-foreground underline-offset-2 hover:underline"
        >
          {part.replace(/^https:\/\/github\.com\/ellipse-software\/bast\//, "")}
        </a>
      );
    }
    if (/^#[0-9]+$/.test(part)) {
      return (
        <a
          key={index}
          href={`https://github.com/ellipse-software/bast/pull/${part.slice(1)}`}
          target="_blank"
          rel="noopener noreferrer"
          className="text-foreground underline-offset-2 hover:underline"
        >
          {part}
        </a>
      );
    }
    return <span key={index}>{part.replace(/\*\*/g, "")}</span>;
  });
}

export default async function ChangelogPage() {
  const [version, releases] = await Promise.all([
    getLatestBastVersion(),
    getBastReleases(40),
  ]);

  return (
    <MarketingShell version={version}>
      <MarketingBreadcrumb label="Changelog" />
      <h1 className="mb-4 text-3xl font-medium tracking-tight sm:text-4xl">
        Changelog
      </h1>
      <p className="mb-10 max-w-2xl text-base leading-relaxed text-muted sm:text-lg">
        Bast releases from{" "}
        <a
          href={bastReleasesUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="text-foreground underline-offset-2 hover:underline"
        >
          GitHub
        </a>
        .
      </p>

      {releases.length === 0 ? (
        <p className="text-sm text-muted">
          Could not load releases right now. Check{" "}
          <a
            href={bastReleasesUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="text-foreground underline-offset-2 hover:underline"
          >
            GitHub
          </a>
          .
        </p>
      ) : (
        <div className="bg-border p-px">
          <div className="divide-y divide-border bg-background">
            {releases.map((release) => (
              <article key={release.tag} className="px-4 py-5 sm:px-5 sm:py-6">
                <header className="mb-3 flex flex-wrap items-baseline gap-x-3 gap-y-1">
                  <h2 className="text-base font-medium tracking-tight text-foreground">
                    <a
                      href={release.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="transition-colors hover:text-accent"
                    >
                      {release.name}
                    </a>
                  </h2>
                  <span className="font-mono text-xs text-muted">
                    {release.tag}
                  </span>
                  {release.publishedAt ? (
                    <time
                      dateTime={release.publishedAt}
                      className="text-xs text-muted"
                    >
                      {formatDate(release.publishedAt)}
                    </time>
                  ) : null}
                </header>
                <ReleaseBody body={release.body} />
              </article>
            ))}
          </div>
        </div>
      )}

      <p className="mt-8 text-sm text-muted">
        Install the latest with the command on the{" "}
        <Link
          href="/"
          className="text-foreground underline-offset-2 hover:underline"
        >
          homepage
        </Link>
        .
      </p>
    </MarketingShell>
  );
}
