import { createVercelAdapter } from "@flags-sdk/vercel";
import { createClient, type FlagsClient } from "@vercel/flags-core";
import { getVercelOidcToken } from "@vercel/oidc";

/**
 * How long a successfully loaded datafile is reused before the next request
 * fetches again. Matches the SDK's default polling interval.
 *
 * The default Vercel flags client keeps an SSE stream open (`stream: true`)
 * and calls `resolveToken()` on every reconnect. With no `FLAGS` SDK key that
 * resolves `getVercelOidcToken()`, which reads the current request's
 * `x-vercel-oidc-token` header. After the first datafile arrives, a reconnect
 * that runs without that request context fails, and the SDK retries until it
 * logs `@vercel/flags-core: Max retry count exceeded`. Page renders keep using
 * the cached datafile, so the request itself does not 500.
 *
 * This client never starts that stream or the background poller. It fetches
 * during a request, while OIDC is available, and serves the last good datafile
 * if a later refresh cannot authenticate.
 */
export const FLAG_DATAFILE_MAX_AGE_MS = 30_000;

const OFFLINE_LIFECYCLE = {
  stream: false as const,
  polling: false as const,
};

const fetchFailures = new WeakMap<FlagsClient, Error>();

function readProcessFlagsKey(): string | undefined {
  const value = process.env.FLAGS;
  if (typeof value !== "string" || value.length === 0) return undefined;
  return value;
}

function requestUrl(input: RequestInfo | URL): string {
  if (typeof input === "string") return input;
  if (input instanceof URL) return input.href;
  return input.url;
}

function defaultCreate(
  sdkKey: string | undefined,
  lifecycle: typeof OFFLINE_LIFECYCLE,
): FlagsClient {
  const created: { client?: FlagsClient } = {};
  const recordDatafileFailure = (error: Error) => {
    if (created.client) fetchFailures.set(created.client, error);
  };
  const client = createClient(sdkKey, {
    ...lifecycle,
    fetch: (async (input, init) => {
      const url = requestUrl(input);
      try {
        const response = await fetch(input, init);
        if (!response.ok && url.includes("/v1/datafile")) {
          recordDatafileFailure(
            new Error(
              `@vercel/flags-core: Failed to fetch flag datafile (${response.status} ${response.statusText}).`,
            ),
          );
        }
        return response;
      } catch (error) {
        if (url.includes("/v1/datafile")) {
          recordDatafileFailure(
            error instanceof Error ? error : new Error(String(error)),
          );
        }
        throw error;
      }
    }) as typeof fetch,
  });
  created.client = client;
  return client;
}

function oidcUnavailable(cause: unknown): Error {
  const error = new Error(
    "@vercel/flags-core: Failed to get OIDC token. Are you running in a Vercel Environment where OIDC tokens are available? Did you mean to use an SDK Key instead? Use the environment variable FLAGS or pass it directly to the client.",
    cause instanceof Error ? { cause } : undefined,
  );
  error.name = "OidcTokenError";
  return error;
}

export type RequestScopedFlagsClientOptions = {
  now?: () => number;
  freshForMs?: number;
  readSdkKey?: () => string | undefined;
  create?: (
    sdkKey: string | undefined,
    lifecycle: typeof OFFLINE_LIFECYCLE,
  ) => FlagsClient;
};

export function createRequestScopedFlagsClient(
  options: RequestScopedFlagsClientOptions = {},
): FlagsClient {
  const now = options.now ?? Date.now;
  const freshForMs = options.freshForMs ?? FLAG_DATAFILE_MAX_AGE_MS;
  const readSdkKey = options.readSdkKey ?? readProcessFlagsKey;
  const create = options.create ?? defaultCreate;

  let current: { client: FlagsClient; at: number } | undefined;
  let metadata: FlagsClient | undefined;
  let pending: Promise<FlagsClient> | null = null;

  async function refresh(): Promise<FlagsClient> {
    const sdkKey = readSdkKey();
    if (!sdkKey) {
      try {
        await getVercelOidcToken();
      } catch (cause) {
        const error = oidcUnavailable(cause);
        if (current) {
          console.warn(
            "@bast/web: feature flag refresh failed; serving the last successful datafile.",
            error,
          );
          return current.client;
        }
        throw error;
      }
    }

    const client = create(sdkKey, OFFLINE_LIFECYCLE);
    try {
      await client.initialize();
    } catch (error) {
      const diagnosed = fetchFailures.get(client) ?? error;
      await client.shutdown();
      if (current) {
        console.warn(
          "@bast/web: feature flag refresh failed; serving the last successful datafile.",
          diagnosed,
        );
        return current.client;
      }
      throw diagnosed;
    }

    const previous = current?.client;
    current = { client, at: now() };
    if (previous) void previous.shutdown();
    return client;
  }

  function acquire(): Promise<FlagsClient> {
    const at = now();
    if (current && at - current.at < freshForMs) {
      return Promise.resolve(current.client);
    }
    if (!pending) {
      pending = refresh().finally(() => {
        pending = null;
      });
    }
    return pending;
  }

  return {
    get origin() {
      metadata ??= create(readSdkKey(), OFFLINE_LIFECYCLE);
      return metadata.origin;
    },
    async evaluate(flagKey, defaultValue, entities) {
      const client = await acquire();
      return client.evaluate(flagKey, defaultValue, entities);
    },
    async bulkEvaluate(flags, entities) {
      const client = await acquire();
      return client.bulkEvaluate(flags, entities);
    },
    async initialize() {
      await acquire();
    },
    async shutdown() {
      const clients = [current?.client, metadata].filter(
        (client): client is FlagsClient => client !== undefined,
      );
      current = undefined;
      metadata = undefined;
      await Promise.all(clients.map((client) => client.shutdown()));
    },
    async getDatafile() {
      const client = await acquire();
      return client.getDatafile();
    },
    async getFallbackDatafile() {
      const client = await acquire();
      return client.getFallbackDatafile();
    },
  };
}

const runtimeFlagsClient = createRequestScopedFlagsClient();

export function runtimeVercelAdapter<ValueType, EntitiesType = unknown>() {
  return createVercelAdapter(runtimeFlagsClient)<ValueType, EntitiesType>();
}

export async function shutdownRuntimeFlagsClient(): Promise<void> {
  await runtimeFlagsClient.shutdown();
}
