const VERSION_PATTERN =
  /^(dev|v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?|nightly\.[0-9]{8}\.[0-9a-fA-F]+)$/;

export const MAX_VERSION = 128;

const CONTEXTS = ["tui", "cli", "panic", "connect_prepare"] as const;

export type ErrorReportContext = (typeof CONTEXTS)[number];

export type ErrorReportPayload = {
  message: string;
  version: string;
  os: string;
  arch: string;
  source: "cli";
  code?: string;
  stack?: string;
  context?: ErrorReportContext;
  command?: string;
};

const MAX_MESSAGE = 4 * 1024;
const MAX_STACK = 16 * 1024;
const MAX_CODE = 64;
const MAX_CONTEXT = 128;
const MAX_COMMAND = 128;

function isContext(value: string): value is ErrorReportContext {
  return (CONTEXTS as readonly string[]).includes(value);
}

function truncate(value: string, max: number): string {
  if (value.length <= max) {
    return value;
  }
  return value.slice(0, max);
}

export function parseErrorReportPayload(body: unknown): ErrorReportPayload | null {
  if (!body || typeof body !== "object") {
    return null;
  }

  const record = body as Record<string, unknown>;
  const { message, version, os, arch, source, code, stack, context, command } =
    record;

  if (
    typeof message !== "string" ||
    message.trim() === "" ||
    message.length > MAX_MESSAGE ||
    typeof version !== "string" ||
    version.length > MAX_VERSION ||
    !VERSION_PATTERN.test(version) ||
    typeof os !== "string" ||
    !/^(darwin|linux)$/.test(os) ||
    typeof arch !== "string" ||
    !/^(arm64|amd64)$/.test(arch) ||
    source !== "cli"
  ) {
    return null;
  }

  if (code !== undefined && (typeof code !== "string" || code.length > MAX_CODE)) {
    return null;
  }
  if (
    stack !== undefined &&
    (typeof stack !== "string" || stack.length > MAX_STACK)
  ) {
    return null;
  }
  if (
    context !== undefined &&
    (typeof context !== "string" ||
      context.length > MAX_CONTEXT ||
      !isContext(context))
  ) {
    return null;
  }
  if (
    command !== undefined &&
    (typeof command !== "string" || command.length > MAX_COMMAND)
  ) {
    return null;
  }

  return {
    message: truncate(message, MAX_MESSAGE),
    version,
    os,
    arch,
    source: "cli",
    ...(typeof code === "string" ? { code } : {}),
    ...(typeof stack === "string" ? { stack: truncate(stack, MAX_STACK) } : {}),
    ...(typeof context === "string" && isContext(context) ? { context } : {}),
    ...(typeof command === "string" ? { command } : {}),
  };
}
