import { describe, expect, test } from "bun:test";

import {
  allowIngest,
  clientIP,
  ERROR_MAX_BYTES,
  readJsonBody,
  TELEMETRY_MAX_BYTES,
  type IngestCounter,
} from "@/lib/ingest-limit";

function memoryCounter(): IngestCounter & { counts: Map<string, number> } {
  const counts = new Map<string, number>();
  return {
    counts,
    async incr(key) {
      const next = (counts.get(key) ?? 0) + 1;
      counts.set(key, next);
      return next;
    },
  };
}

function jsonRequest(body: string, headers: Record<string, string> = {}) {
  return new Request("https://bast.sh/api/telemetry", {
    method: "POST",
    headers: { "content-type": "application/json", ...headers },
    body,
  });
}

describe("clientIP", () => {
  test("uses the first forwarded hop", () => {
    const headers = new Headers({
      "x-forwarded-for": "203.0.113.4, 10.0.0.1",
      "x-real-ip": "198.51.100.8",
    });
    expect(clientIP(headers)).toBe("203.0.113.4");
  });

  test("falls back to a shared bucket for a missing or odd address", () => {
    expect(clientIP(new Headers())).toBe("unknown");
    expect(clientIP(new Headers({ "x-real-ip": "not an ip" }))).toBe("unknown");
  });
});

describe("readJsonBody", () => {
  test("parses a small JSON body", async () => {
    const result = await readJsonBody(
      jsonRequest('{"event":"install"}'),
      TELEMETRY_MAX_BYTES,
    );
    expect(result).toEqual({ ok: true, body: { event: "install" } });
  });

  test("rejects a declared length over the cap", async () => {
    const result = await readJsonBody(
      jsonRequest("{}", { "content-length": String(TELEMETRY_MAX_BYTES + 1) }),
      TELEMETRY_MAX_BYTES,
    );
    expect(result).toEqual({ ok: false, reason: "too_large" });
  });

  test("rejects a stream larger than the cap when length is omitted", async () => {
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new Uint8Array(ERROR_MAX_BYTES + 1));
        controller.close();
      },
    });
    const request = new Request("https://bast.sh/api/errors", {
      method: "POST",
      body: stream,
    });
    const result = await readJsonBody(request, ERROR_MAX_BYTES);
    expect(result).toEqual({ ok: false, reason: "too_large" });
  });

  test("rejects an empty or non-JSON body", async () => {
    expect(await readJsonBody(jsonRequest(""), TELEMETRY_MAX_BYTES)).toEqual({
      ok: false,
      reason: "invalid_json",
    });
    expect(await readJsonBody(jsonRequest("nope"), TELEMETRY_MAX_BYTES)).toEqual({
      ok: false,
      reason: "invalid_json",
    });
  });
});

describe("allowIngest", () => {
  test("stops after the limit and shares one key per IP", async () => {
    const counter = memoryCounter();
    const options = {
      bucket: "telemetry" as const,
      ip: "203.0.113.9",
      limit: 2,
      windowSeconds: 60,
      counter,
    };
    expect(await allowIngest(options)).toBe(true);
    expect(await allowIngest(options)).toBe(true);
    expect(await allowIngest(options)).toBe(false);
    expect(counter.counts.get("ingest:telemetry:203.0.113.9")).toBe(3);
  });

  test("allows the request when the counter throws", async () => {
    const allowed = await allowIngest({
      bucket: "errors",
      ip: "unknown",
      limit: 1,
      windowSeconds: 60,
      counter: {
        async incr() {
          throw new Error("redis down");
        },
      },
    });
    expect(allowed).toBe(true);
  });

  test("allows the request when no counter is configured", async () => {
    expect(
      await allowIngest({
        bucket: "errors",
        ip: "unknown",
        limit: 1,
        windowSeconds: 60,
        counter: null,
      }),
    ).toBe(true);
  });
});
