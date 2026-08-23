import { markdownResponseHeaders } from "@/lib/accept";
import { getLLMText } from "@/lib/llms";
import {
  markdownNotFound,
  staticMarkdownForPath,
} from "@/lib/page-markdown";
import { source } from "@/lib/source";

function normalizePath(pathname: string): string {
  if (!pathname || pathname === "/") return "/";
  const withSlash = pathname.startsWith("/") ? pathname : `/${pathname}`;
  return withSlash.length > 1 && withSlash.endsWith("/")
    ? withSlash.slice(0, -1)
    : withSlash;
}

export const markdownHeaders = markdownResponseHeaders;

export async function markdownForPath(
  pathname: string,
): Promise<{ body: string; status: number }> {
  const path = normalizePath(pathname);
  const staticBody = staticMarkdownForPath(path);
  if (staticBody) {
    return { body: staticBody, status: 200 };
  }

  if (path === "/docs" || path.startsWith("/docs/")) {
    const slugs = path === "/docs" ? [] : path.slice("/docs/".length).split("/");
    const page = source.getPage(slugs);
    if (page) {
      return { body: await getLLMText(page), status: 200 };
    }
  }

  return { body: markdownNotFound(path), status: 404 };
}
