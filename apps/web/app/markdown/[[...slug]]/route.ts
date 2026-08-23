import { markdownForPath, markdownHeaders } from "@/lib/markdown";

export const revalidate = 300;

export async function GET(
  _request: Request,
  props: { params: Promise<{ slug?: string[] }> },
) {
  const { slug = [] } = await props.params;
  const pathname = slug.length === 0 ? "/" : `/${slug.join("/")}`;
  const { body, status } = await markdownForPath(pathname);
  return new Response(body, {
    status,
    headers: markdownHeaders(),
  });
}
