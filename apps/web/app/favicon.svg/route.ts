import { readFileSync } from "node:fs";
import { join } from "node:path";

import { faviconAccent, renderFaviconSvg } from "@/lib/favicon";

function faviconSource(): string {
  const candidates = [
    join(process.cwd(), "lib/favicon-mark.svg"),
    join(process.cwd(), "apps/web/lib/favicon-mark.svg"),
  ];
  for (const path of candidates) {
    try {
      return readFileSync(path, "utf8");
    } catch {
      // Try the next layout (Vercel root vs repo root).
    }
  }
  throw new Error("favicon-mark.svg is missing");
}

export function GET() {
  const body = renderFaviconSvg(faviconSource(), faviconAccent());
  return new Response(body, {
    headers: {
      "Content-Type": "image/svg+xml; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
}
