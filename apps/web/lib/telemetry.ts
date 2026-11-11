import { MAX_VERSION, OS_PATTERN } from "./errors";

const VERSION_PATTERN =
  /^(dev|v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?|nightly\.[0-9]{8}\.[0-9a-fA-F]+)$/;

const INSTALLER_EVENTS = ["install", "update", "up_to_date"] as const;

const CORE_CLI_EVENTS = [
  "tui_open",
  "connect",
  "direct_connect",
  "update",
  "doctor",
  "onboarding_show",
  "onboarding_continue",
  "onboarding_skip",
  "onboarding_vault",
  "onboarding_sync",
  "files_connect",
  "files_shell",
  "vault_link",
  "vault_link_fail",
  "vault_sync",
  "vault_sync_fail",
  "vault_push",
  "vault_push_fail",
  "vault_pull",
  "vault_pull_fail",
  "vault_unlock",
  "vault_logout",
  "vault_passphrase_rotate",
  "vault_passphrase_reset",
] as const;

const SYNC_PROVIDERS = [
  "gcp",
  "aws",
  "azure",
  "boat",
  "upstash",
  "vercel",
  "hetzner",
  "bitqate",
  "railway",
  "fly",
  "digitalocean",
  "boxd",
] as const;

const LIFECYCLE_ACTIONS = {
  boat: ["new", "fork", "stop", "resume", "delete"],
  upstash: ["new", "fork", "stop", "resume", "delete", "key"],
  vercel: ["new", "fork", "stop", "resume", "delete", "cleanup", "token"],
  fly: ["new", "fork", "stop", "resume", "delete"],
  boxd: ["new", "fork", "delete", "start", "stop", "restart"],
  railway: ["new", "stop", "resume", "delete", "key"],
  hetzner: ["start", "stop", "restart", "key"],
  digitalocean: ["new", "fork", "stop", "resume", "delete"],
  bitqate: ["list", "start", "stop", "restart", "key"],
} as const;

function syncEvents(): string[] {
  const events: string[] = [];
  for (const provider of SYNC_PROVIDERS) {
    events.push(
      `sync_${provider}`,
      `sync_${provider}_fail`,
      `sync_${provider}_disable`,
      `sync_${provider}_auto`,
    );
  }
  return events;
}

function lifecycleEvents(): string[] {
  const events: string[] = [];
  for (const [name, actions] of Object.entries(LIFECYCLE_ACTIONS)) {
    for (const action of actions) {
      events.push(`${name}_${action}`, `${name}_${action}_fail`);
    }
  }
  return events;
}

/**
 * Closed set of CLI events. telemetry-events.test.ts fails when apps/bast
 * sends a name that is not in this set.
 */
export const CLI_TELEMETRY_EVENTS: ReadonlySet<string> = new Set([
  ...CORE_CLI_EVENTS,
  ...syncEvents(),
  ...lifecycleEvents(),
]);

export type InstallerTelemetryEvent = (typeof INSTALLER_EVENTS)[number];
export type TelemetrySource = "installer" | "cli";

export type TelemetryPayload = {
  event: string;
  version: string;
  os: string;
  arch: string;
  source: TelemetrySource;
};

function isTelemetryEvent(source: TelemetrySource, event: string): boolean {
  if (source === "installer") {
    return INSTALLER_EVENTS.includes(event as InstallerTelemetryEvent);
  }

  return CLI_TELEMETRY_EVENTS.has(event);
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
    !OS_PATTERN.test(os) ||
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
