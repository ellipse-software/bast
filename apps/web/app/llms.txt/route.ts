import { getLlmsIndex } from "@/lib/llms";

export const revalidate = 300;

export async function GET() {
  return new Response(await getLlmsIndex(), {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
    },
  });
}
