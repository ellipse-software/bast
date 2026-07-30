import { getLLMText, getPageMarkdownUrl } from "@/lib/llms";
import { source } from "@/lib/source";
import { notFound } from "next/navigation";

export const revalidate = false;

export async function GET(
  _req: Request,
  props: RouteContext<"/llms.mdx/docs/[[...slug]]">,
) {
  const { slug } = await props.params;
  if (slug?.at(-1) !== "index.md") notFound();
  const page = source.getPage(slug?.slice(0, -1));
  if (!page) notFound();

  return new Response(await getLLMText(page), {
    headers: {
      "Content-Type": "text/markdown; charset=utf-8",
    },
  });
}

export function generateStaticParams() {
  return source.getPages().map((page) => ({
    slug: getPageMarkdownUrl(page).segments,
  }));
}
