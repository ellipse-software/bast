const PRODUCES = ["text/html", "text/markdown"] as const;

export type ProducedType = (typeof PRODUCES)[number];

type AcceptEntry = { type: string; q: number; specificity: number };

const SKIP_EXTENSION =
  /\.(?:png|jpe?g|gif|svg|ico|webp|ps1|sh|woff2?|txt|md|json|xml|map|css|js)$/i;

const SKIP_PATHS = new Set([
  "/install",
  "/install-nightly",
  "/install-skill",
  "/install-skill.sh",
  "/robots.txt",
  "/sitemap.xml",
  "/openapi.json",
  "/favicon.svg",
  "/bast.skill.md",
  "/llms.txt",
  "/llms-full.txt",
]);

function parseAccept(header: string): AcceptEntry[] {
  return header.split(",").map((raw) => {
    const parts = raw
      .trim()
      .split(";")
      .map((token) => token.trim());
    const type = (parts[0] ?? "").toLowerCase();
    let q = 1;
    for (const param of parts.slice(1)) {
      const [name, value] = param.split("=").map((token) => token.trim());
      if (name === "q") {
        const parsed = Number(value);
        if (!Number.isNaN(parsed)) q = Math.max(0, Math.min(1, parsed));
      }
    }
    const specificity = type === "*/*" ? 0 : type.endsWith("/*") ? 1 : 2;
    return { type, q, specificity };
  });
}

function matches(entry: AcceptEntry, candidate: string): boolean {
  if (entry.type === "*/*") return true;
  if (entry.type.endsWith("/*")) {
    return candidate.startsWith(entry.type.slice(0, -1));
  }
  return entry.type === candidate;
}

/** RFC 9110 content negotiation over the types this site produces. */
export function preferredType(header: string | null): ProducedType | null {
  if (!header) return PRODUCES[0];
  const entries = parseAccept(header);
  if (entries.length === 0) return PRODUCES[0];

  let bestType: ProducedType | null = null;
  let bestQ = -1;
  let bestPosition = Number.POSITIVE_INFINITY;

  for (const candidate of PRODUCES) {
    let matched: AcceptEntry | null = null;
    let matchedPosition = Number.POSITIVE_INFINITY;
    for (let idx = 0; idx < entries.length; idx++) {
      const entry = entries[idx];
      if (!entry || !matches(entry, candidate)) continue;
      if (
        matched === null ||
        entry.specificity > matched.specificity ||
        (entry.specificity === matched.specificity && idx < matchedPosition)
      ) {
        matched = entry;
        matchedPosition = idx;
      }
    }
    if (matched === null) continue;
    if (matched.q <= 0) continue;

    if (
      matched.q > bestQ ||
      (matched.q === bestQ && matchedPosition < bestPosition)
    ) {
      bestQ = matched.q;
      bestPosition = matchedPosition;
      bestType = candidate;
    }
  }

  return bestType;
}

export function appendVaryAccept(headers: Headers): void {
  const existing = headers.get("Vary");
  if (!existing) {
    headers.set("Vary", "Accept");
    return;
  }
  const tokens = existing.split(",").map((token) => token.trim().toLowerCase());
  if (!tokens.includes("accept")) {
    headers.set("Vary", `${existing}, Accept`);
  }
}

export function isRscNavigation(headers: Headers): boolean {
  return (
    headers.has("rsc") ||
    headers.has("next-router-state-tree") ||
    headers.has("next-router-prefetch") ||
    headers.has("next-router-segment-prefetch")
  );
}

export function shouldNegotiateMarkdown(pathname: string): boolean {
  if (pathname.startsWith("/api/")) return false;
  if (pathname.startsWith("/_next/")) return false;
  if (pathname.startsWith("/_vercel/")) return false;
  if (pathname.startsWith("/markdown")) return false;
  if (pathname.startsWith("/llms.mdx/")) return false;
  if (SKIP_PATHS.has(pathname)) return false;
  if (SKIP_EXTENSION.test(pathname)) return false;
  return true;
}

export function markdownRewritePath(pathname: string): string {
  if (pathname === "/") return "/markdown";
  return `/markdown${pathname}`;
}

export function markdownResponseHeaders(): HeadersInit {
  return {
    "Content-Type": "text/markdown; charset=utf-8",
    Vary: "Accept, Accept-Encoding",
    "Cache-Control": "public, s-maxage=60, stale-while-revalidate=86400",
  };
}

export function notAcceptableResponse(): Response {
  return new Response(
    "Not Acceptable\n\nAvailable: text/html, text/markdown\n",
    {
      status: 406,
      headers: {
        "Content-Type": "text/plain; charset=utf-8",
        Vary: "Accept",
      },
    },
  );
}
