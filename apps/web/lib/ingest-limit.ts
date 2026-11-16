import { getRedis, redisConfigured } from "./redis";

export const TELEMETRY_MAX_BYTES = 32 * 1024;
export const ERROR_MAX_BYTES = 24 * 1024;
export const TELEMETRY_RATE_LIMIT = 60;
export const ERROR_RATE_LIMIT = 10;
export const INGEST_WINDOW_SECONDS = 60;

export type IngestBucket = "telemetry" | "errors";

export type IngestCounter = {
  incr(key: string, windowSeconds: number): Promise<number>;
};

export type JsonBodyResult =
  | { ok: true; body: unknown }
  | { ok: false; reason: "too_large" | "invalid_json" };

export function clientIP(headers: Headers): string {
  const forwarded = headers.get("x-forwarded-for")?.split(",")[0]?.trim() ?? "";
  const raw = forwarded || headers.get("x-real-ip")?.trim() || "";
  if (raw.length === 0 || raw.length > 64 || !/^[A-Za-z0-9.:]+$/.test(raw)) {
    return "unknown";
  }
  return raw;
}

export function declaredContentLength(request: Request): number | null {
  const raw = request.headers.get("content-length");
  if (raw === null || raw.trim() === "") {
    return null;
  }
  if (!/^\d+$/.test(raw.trim())) {
    return Number.NaN;
  }
  return Number(raw);
}

export function contentLengthTooLarge(
  request: Request,
  maxBytes: number,
): boolean {
  const declared = declaredContentLength(request);
  return declared !== null && (!Number.isFinite(declared) || declared > maxBytes);
}

async function readLimited(
  request: Request,
  maxBytes: number,
): Promise<Uint8Array | "too_large"> {
  if (!request.body) {
    return new Uint8Array();
  }

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    if (!value) {
      continue;
    }
    total += value.byteLength;
    if (total > maxBytes) {
      await reader.cancel();
      return "too_large";
    }
    chunks.push(value);
  }

  const out = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return out;
}

export async function readJsonBody(
  request: Request,
  maxBytes: number,
): Promise<JsonBodyResult> {
  if (contentLengthTooLarge(request, maxBytes)) {
    return { ok: false, reason: "too_large" };
  }

  const bytes = await readLimited(request, maxBytes);
  if (bytes === "too_large") {
    return { ok: false, reason: "too_large" };
  }
  if (bytes.byteLength === 0) {
    return { ok: false, reason: "invalid_json" };
  }

  try {
    return { ok: true, body: JSON.parse(new TextDecoder().decode(bytes)) };
  } catch {
    return { ok: false, reason: "invalid_json" };
  }
}

function redisIngestCounter(): IngestCounter | null {
  if (!redisConfigured()) {
    return null;
  }
  const redis = getRedis();
  return {
    async incr(key, windowSeconds) {
      const count = Number(await redis.incr(key));
      if (count === 1) {
        await redis.expire(key, windowSeconds);
      } else {
        const ttl = await redis.ttl(key);
        if (typeof ttl === "number" && ttl < 0) {
          await redis.expire(key, windowSeconds);
        }
      }
      return count;
    },
  };
}

/**
 * Allows the request when Redis is not configured or the counter call fails.
 * A missing counter must not drop installer or CLI events.
 */
export async function allowIngest(options: {
  bucket: IngestBucket;
  ip: string;
  limit: number;
  windowSeconds: number;
  counter?: IngestCounter | null;
}): Promise<boolean> {
  const counter =
    options.counter === undefined ? redisIngestCounter() : options.counter;
  if (!counter) {
    return true;
  }

  try {
    const count = await counter.incr(
      `ingest:${options.bucket}:${options.ip}`,
      options.windowSeconds,
    );
    return count <= options.limit;
  } catch {
    return true;
  }
}
