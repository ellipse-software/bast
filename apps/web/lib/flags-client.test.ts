import { afterEach, describe, expect, test } from "bun:test";
import { createClient, type FlagsClient } from "@vercel/flags-core";

import { winget } from "@/flags";
import {
  createRequestScopedFlagsClient,
  shutdownRuntimeFlagsClient,
} from "@/lib/flags-client";

const REQUEST_CONTEXT = Symbol.for("@vercel/request-context");

type RequestContextCarrier = typeof globalThis & {
  [REQUEST_CONTEXT]?: { get?: () => { headers?: Record<string, string> } };
};

function oidcToken(): string {
  const header = Buffer.from(
    JSON.stringify({ alg: "none", typ: "JWT" }),
  ).toString("base64url");
  const payload = Buffer.from(
    JSON.stringify({
      exp: Math.floor(Date.now() / 1000) + 60 * 60,
      project_id: "prj_test",
    }),
  ).toString("base64url");
  return `${header}.${payload}.sig`;
}

function datafile(enabled: boolean) {
  return {
    projectId: "prj_test",
    environment: "production",
    definitions: {
      winget: {
        variants: [false, true],
        environments: {
          production: { fallthrough: enabled ? 1 : 0 },
        },
      },
    },
  };
}

function installRequestContext(token: string | undefined): () => void {
  const target = globalThis as RequestContextCarrier;
  const previousContext = target[REQUEST_CONTEXT];
  const previousOidc = process.env.VERCEL_OIDC_TOKEN;
  const previousFlags = process.env.FLAGS;
  delete process.env.VERCEL_OIDC_TOKEN;
  delete process.env.FLAGS;
  target[REQUEST_CONTEXT] = {
    get() {
      if (!token) return {};
      return { headers: { "x-vercel-oidc-token": token } };
    },
  };
  return () => {
    if (previousContext === undefined) delete target[REQUEST_CONTEXT];
    else target[REQUEST_CONTEXT] = previousContext;
    if (previousOidc === undefined) delete process.env.VERCEL_OIDC_TOKEN;
    else process.env.VERCEL_OIDC_TOKEN = previousOidc;
    if (previousFlags === undefined) delete process.env.FLAGS;
    else process.env.FLAGS = previousFlags;
  };
}

function offlineClient(
  fetchImpl: typeof fetch,
  extra?: { sdkKey?: string },
): FlagsClient {
  return createClient(extra?.sdkKey, {
    stream: false,
    polling: false,
    buildStep: false,
    disableMetrics: true,
    fetch: fetchImpl,
  });
}

describe("request-scoped flags client", () => {
  const restores: Array<() => void> = [];

  afterEach(async () => {
    while (restores.length > 0) restores.pop()?.();
    await shutdownRuntimeFlagsClient();
  });

  test("evaluates a flag from the datafile and does not open a stream", async () => {
    restores.push(installRequestContext(oidcToken()));
    const urls: string[] = [];
    let enabled = true;
    const fetchImpl = (async (input) => {
      const url = String(input);
      urls.push(url);
      if (url.includes("/v1/datafile")) {
        return Response.json(datafile(enabled));
      }
      return new Response(null, { status: 204 });
    }) as typeof fetch;
    let now = 1_000_000;
    const lifecycles: Array<{ stream: false; polling: false }> = [];
    const client = createRequestScopedFlagsClient({
      now: () => now,
      freshForMs: 30_000,
      create: (sdkKey, lifecycle) => {
        lifecycles.push(lifecycle);
        return offlineClient(fetchImpl, { sdkKey });
      },
    });

    const first = await client.evaluate("winget");
    const second = await client.evaluate("winget");
    expect(first.value).toBe(true);
    expect(first.reason).not.toBe("error");
    expect(second.value).toBe(true);
    expect(urls.filter((url) => url.includes("/v1/datafile"))).toHaveLength(1);
    expect(urls.some((url) => url.includes("/v1/stream"))).toBe(false);
    expect(lifecycles).toEqual([{ stream: false, polling: false }]);

    enabled = false;
    now += 30_000;
    const refreshed = await client.evaluate("winget");
    expect(refreshed.value).toBe(false);
    expect(urls.filter((url) => url.includes("/v1/datafile"))).toHaveLength(2);
    expect(urls.some((url) => url.includes("/v1/stream"))).toBe(false);
    await client.shutdown();
  });

  test("keeps the last successful value when a later refresh has no OIDC context", async () => {
    let token: string | undefined = oidcToken();
    restores.push(installRequestContext(token));
    const target = globalThis as RequestContextCarrier;
    const urls: string[] = [];
    const warnings: unknown[][] = [];
    const errors: unknown[][] = [];
    const originalWarn = console.warn;
    const originalError = console.error;
    console.warn = (...args: unknown[]) => {
      warnings.push(args);
    };
    console.error = (...args: unknown[]) => {
      errors.push(args);
    };
    restores.push(() => {
      console.warn = originalWarn;
      console.error = originalError;
    });

    let now = 1_000_000;
    const client = createRequestScopedFlagsClient({
      now: () => now,
      freshForMs: 30_000,
      create: (sdkKey) =>
        offlineClient((async (input) => {
          urls.push(String(input));
          return Response.json(datafile(true));
        }) as typeof fetch, { sdkKey }),
    });

    expect((await client.evaluate("winget")).value).toBe(true);

    token = undefined;
    target[REQUEST_CONTEXT] = { get: () => ({}) };
    now += 30_000;
    const duringOutage = await client.evaluate("winget");
    expect(duringOutage.value).toBe(true);
    expect(warnings).toHaveLength(1);
    expect(String(warnings[0]?.[1])).toContain("Failed to get OIDC token");
    expect(urls.some((url) => url.includes("/v1/stream"))).toBe(false);
    expect(
      errors.some((args) =>
        args.some((arg) => String(arg).includes("Max retry count exceeded")),
      ),
    ).toBe(false);

    token = oidcToken();
    target[REQUEST_CONTEXT] = {
      get: () => ({ headers: { "x-vercel-oidc-token": token } }),
    };
    now += 30_000;
    expect((await client.evaluate("winget")).value).toBe(true);
    await client.shutdown();
  });

  test("reports a missing OIDC token instead of retrying a stream", async () => {
    restores.push(installRequestContext(undefined));
    const urls: string[] = [];
    const errors: unknown[][] = [];
    const originalError = console.error;
    console.error = (...args: unknown[]) => {
      errors.push(args);
    };
    restores.push(() => {
      console.error = originalError;
    });
    const client = createRequestScopedFlagsClient({
      create: (sdkKey) =>
        offlineClient((async (input) => {
          urls.push(String(input));
          return Response.json(datafile(true));
        }) as typeof fetch, { sdkKey }),
    });

    await expect(client.evaluate("winget")).rejects.toThrow(
      /Failed to get OIDC token/,
    );
    expect(urls).toEqual([]);
    expect(
      errors.some((args) =>
        args.some((arg) => String(arg).includes("Max retry count exceeded")),
      ),
    ).toBe(false);
    await client.shutdown();
  });

  test("uses a FLAGS SDK key when one is set and does not require OIDC", async () => {
    restores.push(installRequestContext(undefined));
    const headers: Array<HeadersInit | undefined> = [];
    const client = createRequestScopedFlagsClient({
      readSdkKey: () => "vf_server_test_key",
      create: (sdkKey, lifecycle) => {
        expect(sdkKey).toBe("vf_server_test_key");
        expect(lifecycle).toEqual({ stream: false, polling: false });
        return offlineClient(
          (async (input, init) => {
            const url = String(input);
            if (url.includes("/v1/ingest")) return new Response(null, { status: 204 });
            headers.push(init?.headers);
            expect(url).toContain("/v1/datafile");
            expect(url).not.toContain("/v1/stream");
            return Response.json(datafile(true));
          }) as typeof fetch,
          { sdkKey },
        );
      },
    });

    expect((await client.evaluate("winget")).value).toBe(true);
    expect(JSON.stringify(headers)).toContain("vf_server_test_key");
    await client.shutdown();
  });

  test("surfaces a datafile auth failure from the production client", async () => {
    restores.push(installRequestContext(undefined));
    process.env.FLAGS = "vf_server_test_key";
    restores.push(() => {
      delete process.env.FLAGS;
    });
    const originalFetch = globalThis.fetch;
    const urls: string[] = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      urls.push(String(input));
      expect(new Headers(init?.headers).get("authorization")).toBe(
        "Bearer vf_server_test_key",
      );
      return new Response("nope", { status: 401, statusText: "Unauthorized" });
    }) as typeof fetch;
    restores.push(() => {
      globalThis.fetch = originalFetch;
    });

    const client = createRequestScopedFlagsClient();
    await expect(client.evaluate("winget")).rejects.toThrow(
      /Failed to fetch flag datafile \(401 Unauthorized\)/,
    );
    expect(urls.some((url) => url.includes("/v1/stream"))).toBe(false);
    await client.shutdown();
  });

  test("winget keeps its default and still evaluates through the request-scoped adapter", async () => {
    restores.push(installRequestContext(oidcToken()));
    const originalFetch = globalThis.fetch;
    const urls: string[] = [];
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      const url = String(input);
      urls.push(url);
      if (url.includes("/v1/datafile")) return Response.json(datafile(true));
      return new Response(null, { status: 204 });
    }) as typeof fetch;
    restores.push(() => {
      globalThis.fetch = originalFetch;
    });

    expect(winget.defaultValue).toBe(false);
    expect(winget.origin).toMatchObject({ provider: "vercel" });
    const params = {} as Parameters<NonNullable<typeof winget.decide>>[0];
    await expect(winget.decide?.(params)).resolves.toBe(true);
    expect(urls.some((url) => url.includes("/v1/datafile"))).toBe(true);
    expect(urls.some((url) => url.includes("/v1/stream"))).toBe(false);
    await shutdownRuntimeFlagsClient();
  });

  test("winget decide reports an OIDC miss and leaves the flag default in place", async () => {
    restores.push(installRequestContext(undefined));
    const params = {} as Parameters<NonNullable<typeof winget.decide>>[0];
    await expect(winget.decide?.(params)).rejects.toThrow(
      /Failed to get OIDC token/,
    );
    expect(winget.defaultValue).toBe(false);
  });
});
