import { afterEach, describe, expect, spyOn, test } from "bun:test";

import { aggregateLabel, getStatusPageData } from "@/lib/betterstack";

const TOKEN = "super-secret-token";
const MONITORS = {
  BETTERSTACK_MONITOR_MARKETING: "mon_marketing",
  BETTERSTACK_MONITOR_DOCS: "mon_docs",
  BETTERSTACK_MONITOR_VAULT: "mon_vault",
} as const;

const ENV_KEYS = ["BETTERSTACK_API_TOKEN", ...Object.keys(MONITORS)] as const;

const originalFetch = globalThis.fetch;
const originalTimeout = AbortSignal.timeout;
const originalSetTimeout = globalThis.setTimeout;
const savedEnv = new Map<string, string | undefined>(
  ENV_KEYS.map((key) => [key, process.env[key]]),
);

function restoreEnv(): void {
  for (const key of ENV_KEYS) {
    const value = savedEnv.get(key);
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
  }
}

function setConfiguredEnv(): void {
  process.env.BETTERSTACK_API_TOKEN = TOKEN;
  for (const [key, value] of Object.entries(MONITORS)) {
    process.env[key] = value;
  }
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function monitorBody(status: string) {
  return {
    data: {
      id: "mon",
      attributes: {
        pronounceable_name: "Marketing site",
        status,
        last_checked_at: "2026-10-31T21:00:00.000Z",
        created_at: "2026-01-01T00:00:00.000Z",
      },
    },
  };
}

type FetchInit = RequestInit & { next?: { revalidate?: number } };

describe.serial("betterstack status fetch", () => {
  const warn = spyOn(console, "warn").mockImplementation(() => {});
  const error = spyOn(console, "error").mockImplementation(() => {});
  let releaseHang: ((reason?: unknown) => void) | undefined;

  afterEach(() => {
    releaseHang?.(new Error("test cleanup"));
    releaseHang = undefined;
    warn.mockClear();
    error.mockClear();
    globalThis.fetch = originalFetch;
    AbortSignal.timeout = originalTimeout;
    globalThis.setTimeout = originalSetTimeout;
    restoreEnv();
  });

  test("valid monitor JSON aggregates to operational", async () => {
    setConfiguredEnv();
    const seen: Array<{ url: string; init: FetchInit | undefined }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: FetchInit) => {
      const url = String(input);
      seen.push({ url, init });
      expect(init?.headers).toMatchObject({
        Authorization: `Bearer ${TOKEN}`,
        Accept: "application/json",
      });
      expect(init?.signal).toBeInstanceOf(AbortSignal);
      expect(init?.next?.revalidate).toBe(60);
      if (url.includes("/sla?")) {
        return jsonResponse({
          data: { attributes: { availability: 99.98, total_downtime: 0 } },
        });
      }
      if (url.includes("/incidents?")) {
        return jsonResponse({ data: [], pagination: { next: null } });
      }
      return jsonResponse(monitorBody("up"));
    }) as unknown as typeof fetch;

    const data = await getStatusPageData();

    expect(data.aggregate).toBe("operational");
    expect(aggregateLabel(data.aggregate)).toBe("All systems operational");
    expect(data.configured).toBe(true);
    expect(data.services.every((service) => service.status === "operational")).toBe(
      true,
    );
    expect(seen.length).toBeGreaterThan(0);
    expect(seen.every((call) => call.url.startsWith("https://uptime.betterstack.com"))).toBe(
      true,
    );
    expect(warn).not.toHaveBeenCalled();
    expect(error).not.toHaveBeenCalled();
  });

  test("deriveAggregate stays downtime, maintenance, then degraded", async () => {
    const cases = [
      ["down", "downtime", "Major outage"],
      ["maintenance", "maintenance", "Under maintenance"],
      ["paused", "degraded", "Partial outage"],
    ] as const;

    for (const [monitorStatus, aggregate, label] of cases) {
      setConfiguredEnv();
      globalThis.fetch = (async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/sla?")) {
          return jsonResponse({ data: { attributes: { availability: 99 } } });
        }
        if (url.includes("/incidents?")) return jsonResponse({ data: [] });
        return jsonResponse(monitorBody(monitorStatus));
      }) as unknown as typeof fetch;

      const data = await getStatusPageData();
      expect(data.aggregate).toBe(aggregate);
      expect(aggregateLabel(data.aggregate)).toBe(label);
    }
  });

  test("missing token resolves to the unknown aggregate without fetching", async () => {
    delete process.env.BETTERSTACK_API_TOKEN;
    process.env.BETTERSTACK_MONITOR_MARKETING = MONITORS.BETTERSTACK_MONITOR_MARKETING;
    let called = false;
    globalThis.fetch = (async () => {
      called = true;
      throw new Error("should not fetch");
    }) as unknown as typeof fetch;

    const data = await getStatusPageData();

    expect(called).toBe(false);
    expect(data.configured).toBe(false);
    expect(data.aggregate).toBe("unknown");
    expect(aggregateLabel(data.aggregate)).toBe("Status unavailable");
    expect(warn).not.toHaveBeenCalled();
  });

  test("a timeout error resolves without throwing and does not log the token", async () => {
    setConfiguredEnv();
    globalThis.fetch = (async () => {
      const cause = Object.assign(new Error("Connect Timeout Error"), {
        name: "ConnectTimeoutError",
        code: "UND_ERR_CONNECT_TIMEOUT",
      });
      throw new TypeError("fetch failed", { cause });
    }) as unknown as typeof fetch;

    const data = await getStatusPageData();

    expect(data.aggregate).toBe("degraded");
    expect(data.services.every((service) => service.status === "unknown")).toBe(
      true,
    );
    expect(warn).toHaveBeenCalled();
    expect(error).not.toHaveBeenCalled();
    for (const call of warn.mock.calls) {
      expect(call[0]).toBe("betterstack fetch failed");
      expect(call[1]).toEqual({
        path: expect.any(String),
        reason: "timeout",
      });
      expect(JSON.stringify(call)).not.toContain(TOKEN);
    }
  });

  test("other fetch errors log reason error", async () => {
    setConfiguredEnv();
    globalThis.fetch = (async () => {
      throw new Error("socket hang up");
    }) as unknown as typeof fetch;

    const data = await getStatusPageData();

    expect(data.aggregate).toBe("degraded");
    expect(warn.mock.calls.length).toBeGreaterThan(0);
    expect(warn.mock.calls.every((call) => call[1]?.reason === "error")).toBe(true);
    expect(error).not.toHaveBeenCalled();
  });

  test("a hanging fetch hits the deadline and returns unknown", async () => {
    setConfiguredEnv();
    const timeouts: number[] = [];
    AbortSignal.timeout = ((milliseconds: number) => {
      timeouts.push(milliseconds);
      return originalTimeout.call(AbortSignal, milliseconds);
    }) as typeof AbortSignal.timeout;

    const deadlines: number[] = [];
    globalThis.setTimeout = ((
      handler: TimerHandler,
      milliseconds?: number,
      ...args: unknown[]
    ) => {
      if (milliseconds === 3_500) deadlines.push(milliseconds);
      const delay = milliseconds === 3_500 ? 20 : milliseconds;
      return originalSetTimeout(handler, delay, ...args);
    }) as typeof setTimeout;

    globalThis.fetch = ((_input: RequestInfo | URL, init?: RequestInit) => {
      return new Promise((_resolve, reject) => {
        releaseHang = reject;
        const signal = init?.signal;
        const onAbort = () => {
          reject(
            signal?.reason instanceof Error
              ? signal.reason
              : new DOMException("The operation timed out.", "TimeoutError"),
          );
        };
        if (signal?.aborted) onAbort();
        else signal?.addEventListener("abort", onAbort, { once: true });
      });
    }) as unknown as typeof fetch;

    const data = await getStatusPageData();
    await new Promise((resolve) => originalSetTimeout(resolve, 30));

    expect(data.aggregate).toBe("unknown");
    expect(aggregateLabel(data.aggregate)).toBe("Status unavailable");
    expect(data.services.every((service) => service.status === "unknown")).toBe(
      true,
    );
    expect(timeouts).toContain(4_000);
    expect(deadlines).toEqual([3_500]);
    expect(warn).toHaveBeenCalled();
    expect(
      warn.mock.calls.every(
        (call) =>
          call[0] === "betterstack fetch failed" && call[1]?.reason === "timeout",
      ),
    ).toBe(true);
    expect(JSON.stringify(warn.mock.calls)).not.toContain(TOKEN);
    expect(error).not.toHaveBeenCalled();
  }, 2_000);
});
