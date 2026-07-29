import { getLlmsIndex } from "@/lib/llms";

export const revalidate = false;

export function GET() {
  return new Response(getLlmsIndex(), {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
    },
  });
}
