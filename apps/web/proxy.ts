import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

import {
  appendVaryAccept,
  isRscNavigation,
  markdownRewritePath,
  notAcceptableResponse,
  preferredType,
  shouldNegotiateMarkdown,
} from "@/lib/accept";

export function proxy(request: NextRequest) {
  const pathname = request.nextUrl.pathname;

  if (
    request.method !== "GET" &&
    request.method !== "HEAD" &&
    request.method !== "OPTIONS"
  ) {
    return NextResponse.next();
  }

  if (isRscNavigation(request.headers) || !shouldNegotiateMarkdown(pathname)) {
    const response = NextResponse.next();
    appendVaryAccept(response.headers);
    return response;
  }

  if (pathname.endsWith(".md")) {
    const url = request.nextUrl.clone();
    url.pathname = markdownRewritePath(pathname.slice(0, -3));
    const rewritten = NextResponse.rewrite(url);
    appendVaryAccept(rewritten.headers);
    return rewritten;
  }

  const acceptHeader = request.headers.get("accept");
  const chosen = preferredType(acceptHeader);

  if (chosen === "text/markdown") {
    const url = request.nextUrl.clone();
    url.pathname = markdownRewritePath(pathname);
    const rewritten = NextResponse.rewrite(url);
    appendVaryAccept(rewritten.headers);
    return rewritten;
  }

  if (chosen === null && acceptHeader) {
    return notAcceptableResponse();
  }

  const response = NextResponse.next();
  appendVaryAccept(response.headers);
  return response;
}

export const config = {
  matcher: ["/((?!api/|_next/|_vercel/|markdown/).*)"],
};
