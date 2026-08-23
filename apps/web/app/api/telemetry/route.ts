import { jsonError } from "@/lib/api-error";
import { captureTelemetry } from "@/lib/posthog";
import { parseTelemetryPayload } from "@/lib/telemetry";

function getCountryFromRequest(request: Request): string | undefined {
  const country = request.headers.get("x-vercel-ip-country");
  if (!country || country === "XX" || !/^[A-Z]{2}$/i.test(country)) {
    return undefined;
  }

  return country.toUpperCase();
}

export async function POST(request: Request) {
  let body: unknown;

  try {
    body = await request.json();
  } catch {
    return jsonError(400, {
      code: "invalid_json",
      message: "Request body is not valid JSON.",
      hint: "POST application/json matching the TelemetryPayload schema in https://bast.sh/openapi.json.",
    });
  }

  const payload = parseTelemetryPayload(body);
  if (!payload) {
    return jsonError(400, {
      code: "invalid_payload",
      message: "The telemetry payload was missing or invalid.",
      hint: "Send event, version, os (darwin|linux), arch (arm64|amd64), and source (installer|cli). See https://bast.sh/openapi.json.",
    });
  }

  await captureTelemetry(payload, getCountryFromRequest(request));

  return new Response(null, { status: 204 });
}
