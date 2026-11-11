import { jsonError } from "@/lib/api-error";
import {
  allowIngest,
  clientIP,
  contentLengthTooLarge,
  INGEST_WINDOW_SECONDS,
  readJsonBody,
  TELEMETRY_MAX_BYTES,
  TELEMETRY_RATE_LIMIT,
} from "@/lib/ingest-limit";
import { captureTelemetry } from "@/lib/posthog";
import { parseTelemetryPayload } from "@/lib/telemetry";

function getCountryFromRequest(request: Request): string | undefined {
  const country = request.headers.get("x-vercel-ip-country");
  if (!country || country === "XX" || !/^[A-Z]{2}$/i.test(country)) {
    return undefined;
  }

  return country.toUpperCase();
}

function tooLarge() {
  return jsonError(400, {
    code: "payload_too_large",
    message: "Telemetry payload exceeds the 32 KiB limit.",
    hint: "Send a JSON body of at most 32768 bytes. See https://bast.sh/openapi.json.",
  });
}

export async function POST(request: Request) {
  if (contentLengthTooLarge(request, TELEMETRY_MAX_BYTES)) {
    return tooLarge();
  }

  const allowed = await allowIngest({
    bucket: "telemetry",
    ip: clientIP(request.headers),
    limit: TELEMETRY_RATE_LIMIT,
    windowSeconds: INGEST_WINDOW_SECONDS,
  });
  if (!allowed) {
    return jsonError(429, {
      code: "rate_limited",
      message: "Too many telemetry requests.",
      hint: "Wait a minute and retry. At most 60 events per minute per IP.",
    });
  }

  const parsed = await readJsonBody(request, TELEMETRY_MAX_BYTES);
  if (!parsed.ok) {
    if (parsed.reason === "too_large") {
      return tooLarge();
    }
    return jsonError(400, {
      code: "invalid_json",
      message: "Request body is not valid JSON.",
      hint: "POST application/json matching the TelemetryPayload schema in https://bast.sh/openapi.json.",
    });
  }

  const payload = parseTelemetryPayload(parsed.body);
  if (!payload) {
    return jsonError(400, {
      code: "invalid_payload",
      message: "The telemetry payload was missing or invalid.",
      hint: "Send event, version, os (darwin|linux|windows), arch (arm64|amd64), and source (installer|cli). See https://bast.sh/openapi.json.",
    });
  }

  await captureTelemetry(payload, getCountryFromRequest(request));

  return new Response(null, { status: 204 });
}
