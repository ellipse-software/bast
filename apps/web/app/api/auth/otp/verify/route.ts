import { jsonError } from "@/lib/api-error";
import { verifyEmailOTP } from "@/lib/auth";
import { redisConfigured } from "@/lib/redis";

export async function POST(request: Request) {
  if (!redisConfigured()) {
    return jsonError(503, {
      code: "vault_auth_unconfigured",
      message: "Vault auth is not configured on this origin.",
      hint: "Self-host with Upstash Redis or use https://bast.sh. See https://bast.sh/docs/reference/self-hosting.",
    });
  }
  let body: { email?: string; code?: string };
  try {
    body = await request.json();
  } catch {
    return jsonError(400, {
      code: "invalid_json",
      message: "Request body is not valid JSON.",
      hint: "POST {\"email\":\"you@example.com\",\"code\":\"123456\"}. See https://bast.sh/openapi.json.",
    });
  }
  const email = body.email?.trim();
  const code = body.code?.trim();
  if (!email || !code) {
    return jsonError(400, {
      code: "email_and_code_required",
      message: "email and code are required",
      hint: "Send the same email used in /api/auth/otp/start and the 6-digit code.",
    });
  }
  try {
    const verified = await verifyEmailOTP(email, code);
    return Response.json({
      ok: true,
      token: verified.token,
      userId: verified.userId,
      email: verified.email,
      deviceId: verified.deviceId,
    });
  } catch (error) {
    const status =
      error && typeof error === "object" && "status" in error ? Number(error.status) : 401;
    return jsonError(status || 401, {
      code: "invalid_or_expired_code",
      message: "invalid or expired code",
      hint: "Request a new code with POST /api/auth/otp/start. Codes expire after 10 minutes and have limited attempts.",
    });
  }
}
