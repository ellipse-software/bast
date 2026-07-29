import { PostHog } from "posthog-node";

import type { TelemetryPayload } from "@/lib/telemetry";

let client: PostHog | null = null;

function getPostHogClient(): PostHog | null {
  const apiKey = process.env.POSTHOG_API_KEY;

  if (!apiKey) {
    return null;
  }

  client ??= new PostHog(apiKey, {
    host: process.env.POSTHOG_HOST ?? "https://us.i.posthog.com",
    flushAt: 1,
    flushInterval: 0,
  });

  return client;
}

export async function captureTelemetry(
  payload: TelemetryPayload,
  country?: string,
) {
  const posthog = getPostHogClient();

  if (!posthog) {
    console.info("[telemetry]", { ...payload, country });
    return;
  }

  await posthog.captureImmediate({
    distinctId: "anonymous",
    event: `bast_${payload.event}`,
    properties: {
      version: payload.version,
      os: payload.os,
      arch: payload.arch,
      source: payload.source,
      ...(country ? { country } : {}),
      $process_person_profile: false,
    },
  });
}
