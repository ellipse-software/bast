import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { parseErrorReportPayload } from "@/lib/errors";
import { parseTelemetryPayload } from "@/lib/telemetry";

const installer = readFileSync(
  join(import.meta.dir, "../public/install.ps1"),
  "utf8",
);

function telemetry(overrides: Record<string, unknown> = {}) {
  return parseTelemetryPayload({
    event: "install",
    version: "v0.11.0",
    os: "windows",
    arch: "amd64",
    source: "installer",
    ...overrides,
  });
}

describe("parseTelemetryPayload", () => {
  test("accepts the Windows installer payload", () => {
    expect(installer).toContain('os = "windows"');
    expect(telemetry()).toEqual({
      event: "install",
      version: "v0.11.0",
      os: "windows",
      arch: "amd64",
      source: "installer",
    });
  });

  test("accepts current CLI sync and doctor events", () => {
    expect(
      telemetry({ event: "sync_boxd", os: "darwin", source: "cli" })?.event,
    ).toBe("sync_boxd");
    expect(
      telemetry({ event: "doctor", os: "linux", source: "cli" })?.event,
    ).toBe("doctor");
    expect(
      telemetry({ event: "boxd_start", os: "windows", source: "cli" })?.event,
    ).toBe("boxd_start");
  });

  test("rejects unknown events and operating systems", () => {
    expect(telemetry({ event: "sync_evil", source: "cli" })).toBeNull();
    expect(telemetry({ os: "freebsd" })).toBeNull();
    expect(telemetry({ event: "doctor", source: "installer" })).toBeNull();
  });
});

describe("parseErrorReportPayload", () => {
  test("accepts a Windows error report", () => {
    expect(
      parseErrorReportPayload({
        message: "ssh failed",
        version: "v0.11.0",
        os: "windows",
        arch: "arm64",
        source: "cli",
      }),
    ).toMatchObject({ os: "windows", message: "ssh failed" });
  });

  test("rejects an unknown operating system", () => {
    expect(
      parseErrorReportPayload({
        message: "ssh failed",
        version: "v0.11.0",
        os: "freebsd",
        arch: "arm64",
        source: "cli",
      }),
    ).toBeNull();
  });
});
