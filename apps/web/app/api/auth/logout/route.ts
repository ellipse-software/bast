import { jsonError } from "@/lib/api-error";
import { revokeBearer } from "@/lib/auth";
import { redisConfigured } from "@/lib/redis";

export async function POST(request: Request) {
  if (!redisConfigured()) {
    return jsonError(503, {
      code: "vault_auth_unconfigured",
      message: "Vault auth is not configured on this origin.",
      hint: "Self-host with Upstash Redis or use https://bast.sh. See https://bast.sh/docs/reference/self-hosting.",
    });
  }
  await revokeBearer(request.headers.get("authorization"));
  return Response.json({ ok: true });
}
