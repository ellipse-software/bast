import { revokeBearer } from "@/lib/auth";
import { redisConfigured } from "@/lib/redis";

export async function POST(request: Request) {
  if (!redisConfigured()) {
    return Response.json({ error: "vault auth is not configured" }, { status: 503 });
  }
  await revokeBearer(request.headers.get("authorization"));
  return Response.json({ ok: true });
}
