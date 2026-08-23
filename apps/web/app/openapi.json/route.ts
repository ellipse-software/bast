import { openApiSpec } from "@/lib/openapi";

export const revalidate = 300;

export function GET() {
  return Response.json(openApiSpec, {
    headers: {
      "Content-Type": "application/json; charset=utf-8",
      "Cache-Control": "public, s-maxage=300, stale-while-revalidate=86400",
    },
  });
}
