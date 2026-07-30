import { MAX_VERSION } from "./errors";

const VERSION_PATTERN =
  /^(dev|v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?|nightly\.[0-9]{8}\.[0-9a-fA-F]+)$/;

const INSTALLER_EVENTS = ["install", "update", "up_to_date"] as const;
const CLI_EVENTS = [
  "tui_open",
  "connect",
  "direct_connect",
  "sync_gcp",
  "sync_gcp_fail",
  "sync_gcp_disable",
] as const;

export type InstallerTelemetryEvent = (typeof INSTALLER_EVENTS)[number];
export type CliTelemetryEvent = (typeof CLI_EVENTS)[number];
export type TelemetrySource = "installer" | "cli";
export type TelemetryEvent = InstallerTelemetryEvent | CliTelemetryEvent;

export type TelemetryPayload = {
  event: TelemetryEvent;
  version: string;
  os: string;
  arch: string;
  source: TelemetrySource;
};

function isTelemetryEvent(
  source: TelemetrySource,
  event: string,
): event is TelemetryEvent {
  if (source === "installer") {
    return INSTALLER_EVENTS.includes(event as InstallerTelemetryEvent);
  }

  return CLI_EVENTS.includes(event as CliTelemetryEvent);
}

export function parseTelemetryPayload(body: unknown): TelemetryPayload | null {
  if (!body || typeof body !== "object") {
    return null;
  }

  const record = body as Record<string, unknown>;
  const { event, version, os, arch, source } = record;

  if (
    typeof event !== "string" ||
    typeof version !== "string" ||
    version.length > MAX_VERSION ||
    !VERSION_PATTERN.test(version) ||
    typeof os !== "string" ||
    !/^(darwin|linux)$/.test(os) ||
    typeof arch !== "string" ||
    !/^(arm64|amd64)$/.test(arch) ||
    (source !== "installer" && source !== "cli") ||
    !isTelemetryEvent(source, event)
  ) {
    return null;
  }

  return {
    event,
    version,
    os,
    arch,
    source,
  };
}
