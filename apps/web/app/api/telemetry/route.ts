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
    return new Response(null, { status: 400 });
  }

  const payload = parseTelemetryPayload(body);
  if (!payload) {
    return new Response(null, { status: 400 });
  }

  await captureTelemetry(payload, getCountryFromRequest(request));

  return new Response(null, { status: 204 });
}
