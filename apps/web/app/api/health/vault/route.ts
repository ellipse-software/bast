import { healthResponse, type HealthCheckResult } from "@/lib/health";
import { getVaultObject, r2Configured } from "@/lib/r2";
import { getRedis, redisConfigured } from "@/lib/redis";

export const dynamic = "force-dynamic";

const R2_PROBE_KEY = "healthcheck/probe";

export async function GET() {
  const checks: Record<string, HealthCheckResult> = {
    configured: {
      ok: redisConfigured() && r2Configured(),
      detail:
        redisConfigured() && r2Configured()
          ? "Upstash Redis and Cloudflare R2 env present"
          : "missing UPSTASH_REDIS_* and/or R2_* configuration",
    },
  };

  if (!redisConfigured()) {
    checks.redis = { ok: false, detail: "not configured" };
  } else {
    try {
      const pong = await getRedis().ping();
      checks.redis = {
        ok: pong === "PONG" || pong === "pong" || Boolean(pong),
        detail: typeof pong === "string" ? `ping ${pong}` : "ping ok",
      };
    } catch (error) {
      checks.redis = {
        ok: false,
        detail: error instanceof Error ? error.message : "redis ping failed",
      };
    }
  }

  if (!r2Configured()) {
    checks.r2 = { ok: false, detail: "not configured" };
  } else {
    try {
      // A missing probe object is fine: the request still proves credentials and network.
      await getVaultObject(R2_PROBE_KEY);
      checks.r2 = { ok: true, detail: "bucket reachable" };
    } catch (error) {
      checks.r2 = {
        ok: false,
        detail: error instanceof Error ? error.message : "r2 probe failed",
      };
    }
  }

  return healthResponse("vault", checks);
}
