import { verifyEmailOTP } from "@/lib/auth";
import { redisConfigured } from "@/lib/redis";

export async function POST(request: Request) {
  if (!redisConfigured()) {
    return Response.json({ error: "vault auth is not configured" }, { status: 503 });
  }
  let body: { email?: string; code?: string };
  try {
    body = await request.json();
  } catch {
    return Response.json({ error: "invalid json" }, { status: 400 });
  }
  const email = body.email?.trim();
  const code = body.code?.trim();
  if (!email || !code) {
    return Response.json({ error: "email and code are required" }, { status: 400 });
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
    const message = error instanceof Error ? error.message : "verification failed";
    return Response.json({ error: message }, { status: 401 });
  }
}
