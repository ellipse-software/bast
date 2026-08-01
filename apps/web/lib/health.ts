export type HealthStatus = "healthy" | "degraded" | "unhealthy";

export type HealthCheckResult = {
  ok: boolean;
  detail?: string;
};

export type HealthResponse = {
  ok: boolean;
  service: string;
  status: HealthStatus;
  checks: Record<string, HealthCheckResult>;
  timestamp: string;
};

export function healthResponse(
  service: string,
  checks: Record<string, HealthCheckResult>,
): Response {
  const values = Object.values(checks);
  const allOk = values.every((check) => check.ok);
  const anyOk = values.some((check) => check.ok);
  const status: HealthStatus = allOk
    ? "healthy"
    : anyOk
      ? "degraded"
      : "unhealthy";

  const body: HealthResponse = {
    ok: allOk,
    service,
    status,
    checks,
    timestamp: new Date().toISOString(),
  };

  return Response.json(body, {
    status: allOk ? 200 : 503,
    headers: {
      "Cache-Control": "no-store",
    },
  });
}
