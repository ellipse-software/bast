import type { ComponentProps } from "react";

import { changelogMarkdownRenderer } from "@/lib/changelog-markdown";

const headingClass =
  "mt-4 mb-2 text-sm font-medium tracking-tight text-foreground first:mt-0";
const linkClass = "text-foreground underline-offset-2 hover:underline";

const { MarkdownServer } = changelogMarkdownRenderer;

function isSafeHref(href: string | undefined): href is string {
  if (!href) return false;
  const trimmed = href.trim();
  if (!trimmed) return false;
  if (trimmed.startsWith("/") || trimmed.startsWith("#") || trimmed.startsWith("?")) {
    return !trimmed.startsWith("//");
  }
  try {
    const url = new URL(trimmed);
    return (
      url.protocol === "http:" ||
      url.protocol === "https:" ||
      url.protocol === "mailto:"
    );
  } catch {
    return false;
  }
}

function MarkdownLink({ href, children, ...props }: ComponentProps<"a">) {
  const safeHref = isSafeHref(href) ? href : undefined;
  const external =
    typeof safeHref === "string" && /^(https?:|mailto:)/i.test(safeHref);

  return (
    <a
      {...props}
      href={safeHref}
      className={linkClass}
      {...(external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
    >
      {children}
    </a>
  );
}

function MarkdownImage({ src, alt, ...props }: ComponentProps<"img">) {
  if (typeof src !== "string" || !isSafeHref(src) || !/^https?:/i.test(src)) {
    return null;
  }

  return (
    // GitHub release images are arbitrary remote URLs, not in next/image.
    // eslint-disable-next-line @next/next/no-img-element
    <img
      {...props}
      src={src}
      alt={alt ?? ""}
      className="my-3 max-w-full"
    />
  );
}

const changelogComponents = {
  h1: (props: ComponentProps<"h3">) => (
    <h3 {...props} className={headingClass} />
  ),
  h2: (props: ComponentProps<"h3">) => (
    <h3 {...props} className={headingClass} />
  ),
  h3: (props: ComponentProps<"h4">) => (
    <h4 {...props} className={headingClass} />
  ),
  h4: (props: ComponentProps<"h4">) => (
    <h4 {...props} className={headingClass} />
  ),
  h5: (props: ComponentProps<"h4">) => (
    <h4 {...props} className={headingClass} />
  ),
  h6: (props: ComponentProps<"h4">) => (
    <h4 {...props} className={headingClass} />
  ),
  p: (props: ComponentProps<"p">) => (
    <p {...props} className="my-2 first:mt-0 last:mb-0" />
  ),
  ul: (props: ComponentProps<"ul">) => (
    <ul
      {...props}
      className="my-2 ml-4 list-disc space-y-1 marker:text-foreground/50 first:mt-0 last:mb-0"
    />
  ),
  ol: (props: ComponentProps<"ol">) => (
    <ol
      {...props}
      className="my-2 ml-4 list-decimal space-y-1 marker:text-foreground/50 first:mt-0 last:mb-0"
    />
  ),
  li: (props: ComponentProps<"li">) => (
    <li {...props} className="pl-0.5 [&>p]:my-0" />
  ),
  a: MarkdownLink,
  strong: (props: ComponentProps<"strong">) => (
    <strong {...props} className="font-medium text-foreground" />
  ),
  code: ({ className, ...props }: ComponentProps<"code">) =>
    className ? (
      <code {...props} className={className} />
    ) : (
      <code {...props} className="font-mono text-[13px] text-foreground" />
    ),
  pre: (props: ComponentProps<"pre">) => (
    <pre
      {...props}
      className="my-3 overflow-x-auto bg-surface p-3 font-mono text-xs text-foreground first:mt-0 last:mb-0"
    />
  ),
  blockquote: (props: ComponentProps<"blockquote">) => (
    <blockquote
      {...props}
      className="my-3 border-l-2 border-border pl-3 first:mt-0 last:mb-0"
    />
  ),
  hr: () => <hr className="my-4 border-border" />,
  img: MarkdownImage,
  table: (props: ComponentProps<"table">) => (
    <div className="my-3 overflow-x-auto first:mt-0 last:mb-0">
      <table {...props} className="w-full text-left" />
    </div>
  ),
  th: (props: ComponentProps<"th">) => (
    <th
      {...props}
      className="border-b border-border py-1.5 pr-3 font-medium text-foreground"
    />
  ),
  td: (props: ComponentProps<"td">) => (
    <td {...props} className="border-b border-border py-1.5 pr-3" />
  ),
};

export function ReleaseBody({ body }: { body: string }) {
  if (!body) {
    return (
      <p className="text-sm text-muted">No release notes for this tag.</p>
    );
  }

  return (
    <div className="text-sm leading-relaxed text-muted">
      <MarkdownServer components={changelogComponents}>{body}</MarkdownServer>
    </div>
  );
}
