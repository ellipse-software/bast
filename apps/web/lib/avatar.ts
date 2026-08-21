import { createHash } from "node:crypto";

import { getObject, putObject, r2Configured } from "@/lib/r2";
import { getRedis, redisConfigured } from "@/lib/redis";
import { normalizeXHandle } from "@/lib/x-handle";
import { getXProfile } from "@/lib/x-profile";

export type AvatarImage = {
  body: Uint8Array;
  contentType: string;
  etag: string;
};

type AvatarMeta = {
  status: "ok" | "missing";
  contentType?: string;
  etag?: string;
  fetchedAt: number;
  source?: string;
};

const META_PREFIX = "avatar:v1:";
const LOCK_PREFIX = "avatar:lock:v1:";
const R2_PREFIX = "avatars/";

const FRESH_MS = 7 * 24 * 60 * 60 * 1000;
const MISSING_MS = 6 * 60 * 60 * 1000;
const OK_REDIS_TTL_SEC = 30 * 24 * 60 * 60;
const MISSING_REDIS_TTL_SEC = 6 * 60 * 60;
const LOCK_SEC = 20;
const MAX_BYTES = 2 * 1024 * 1024;
const FETCH_MS = 8_000;

const FETCH_HEADERS = {
  Accept: "image/jpeg,image/png,image/webp,image/gif,*/*",
  "User-Agent": "bast.sh",
} as const;

const ALLOWED_IMAGE_HOSTS = new Set([
  "pbs.twimg.com",
  "abs.twimg.com",
  "unavatar.io",
]);

const ALLOWED_TYPES = new Set([
  "image/jpeg",
  "image/jpg",
  "image/png",
  "image/webp",
  "image/gif",
]);

export function isAllowedAvatarUrl(url: string): boolean {
  try {
    const parsed = new URL(url);
    return parsed.protocol === "https:" && ALLOWED_IMAGE_HOSTS.has(parsed.hostname);
  } catch {
    return false;
  }
}

function cacheConfigured(): boolean {
  return redisConfigured() && r2Configured();
}

function metaKey(handle: string): string {
  return `${META_PREFIX}${handle}`;
}

function lockKey(handle: string): string {
  return `${LOCK_PREFIX}${handle}`;
}

function r2Key(handle: string): string {
  return `${R2_PREFIX}${handle}`;
}

function etagFor(body: Uint8Array): string {
  return createHash("sha256").update(body).digest("hex").slice(0, 16);
}

function isFresh(meta: AvatarMeta): boolean {
  const age = Date.now() - meta.fetchedAt;
  if (meta.status === "missing") return age < MISSING_MS;
  return age < FRESH_MS;
}

async function readMeta(handle: string): Promise<AvatarMeta | null> {
  if (!redisConfigured()) return null;
  const raw = await getRedis().get<AvatarMeta | string>(metaKey(handle));
  if (!raw) return null;
  try {
    return typeof raw === "string" ? (JSON.parse(raw) as AvatarMeta) : raw;
  } catch {
    return null;
  }
}

async function writeMeta(handle: string, meta: AvatarMeta): Promise<void> {
  if (!redisConfigured()) return;
  const ttl = meta.status === "missing" ? MISSING_REDIS_TTL_SEC : OK_REDIS_TTL_SEC;
  await getRedis().set(metaKey(handle), meta, { ex: ttl });
}

async function downloadImage(
  url: string,
): Promise<{ body: Uint8Array; contentType: string } | null> {
  if (!isAllowedAvatarUrl(url)) return null;

  const response = await fetch(url, {
    headers: FETCH_HEADERS,
    redirect: "follow",
    signal: AbortSignal.timeout(FETCH_MS),
  });
  if (!response.ok) return null;
  if (!isAllowedAvatarUrl(response.url)) return null;

  const rawType = response.headers.get("content-type")?.split(";")[0]?.trim().toLowerCase();
  if (!rawType || !ALLOWED_TYPES.has(rawType)) return null;
  const contentType = rawType === "image/jpg" ? "image/jpeg" : rawType;

  const body = new Uint8Array(await response.arrayBuffer());
  if (body.byteLength === 0 || body.byteLength > MAX_BYTES) return null;
  return { body, contentType };
}

async function fetchFromFxTwitter(
  handle: string,
): Promise<{ body: Uint8Array; contentType: string; source: string } | null> {
  const profile = await getXProfile(handle);
  if (!profile?.avatarUrl) return null;
  const image = await downloadImage(profile.avatarUrl);
  if (!image) return null;
  return { ...image, source: "fxtwitter" };
}

async function fetchFromUnavatar(
  handle: string,
): Promise<{ body: Uint8Array; contentType: string; source: string } | null> {
  const image = await downloadImage(
    `https://unavatar.io/x/${handle}?fallback=false`,
  );
  if (!image) return null;
  return { ...image, source: "unavatar" };
}

async function fetchFromSource(
  handle: string,
): Promise<{ body: Uint8Array; contentType: string; source: string } | null> {
  try {
    const fx = await fetchFromFxTwitter(handle);
    if (fx) return fx;
  } catch {
    // Try the next source.
  }
  try {
    return await fetchFromUnavatar(handle);
  } catch {
    return null;
  }
}

async function persist(
  handle: string,
  image: { body: Uint8Array; contentType: string; source: string },
): Promise<AvatarImage> {
  const etag = etagFor(image.body);
  if (cacheConfigured()) {
    await putObject(r2Key(handle), image.body, image.contentType);
    await writeMeta(handle, {
      status: "ok",
      contentType: image.contentType,
      etag,
      fetchedAt: Date.now(),
      source: image.source,
    });
  }
  return { body: image.body, contentType: image.contentType, etag };
}

async function readCached(handle: string): Promise<AvatarImage | null> {
  if (!cacheConfigured()) return null;
  const meta = await readMeta(handle);
  if (!meta || meta.status !== "ok" || !meta.contentType || !meta.etag) {
    return null;
  }
  const body = await getObject(r2Key(handle));
  if (!body) return null;
  return { body, contentType: meta.contentType, etag: meta.etag };
}

async function refresh(handle: string): Promise<AvatarImage | null> {
  const image = await fetchFromSource(handle);
  if (!image) {
    await writeMeta(handle, { status: "missing", fetchedAt: Date.now() });
    return null;
  }
  return persist(handle, image);
}

async function withLock<T>(
  handle: string,
  task: () => Promise<T>,
  onBusy: () => Promise<T | null>,
): Promise<T | null> {
  if (!redisConfigured()) return task();

  const acquired = await getRedis().set(lockKey(handle), 1, {
    nx: true,
    ex: LOCK_SEC,
  });
  if (acquired) {
    try {
      return await task();
    } finally {
      await getRedis().del(lockKey(handle));
    }
  }

  await new Promise((resolve) => setTimeout(resolve, 400));
  const cached = await onBusy();
  if (cached) return cached;
  return task();
}

export async function getAvatar(
  username: string,
): Promise<AvatarImage | "missing" | null> {
  const handle = normalizeXHandle(username);
  if (!handle) return null;

  try {
    const meta = await readMeta(handle);
    if (meta?.status === "missing" && isFresh(meta)) return "missing";

    if (meta?.status === "ok") {
      const cached = await readCached(handle);
      if (cached) {
        return cached;
      }
    }

    const fetched = await withLock(
      handle,
      () => refresh(handle),
      () => readCached(handle),
    );
    return fetched ?? (meta?.status === "missing" ? "missing" : null);
  } catch {
    const image = await fetchFromSource(handle);
    if (!image) return null;
    return {
      body: image.body,
      contentType: image.contentType,
      etag: etagFor(image.body),
    };
  }
}

export async function avatarIsStale(username: string): Promise<boolean> {
  const handle = normalizeXHandle(username);
  if (!handle) return false;
  try {
    const meta = await readMeta(handle);
    if (!meta || meta.status !== "ok") return false;
    return !isFresh(meta);
  } catch {
    return false;
  }
}

export async function revalidateAvatar(username: string): Promise<void> {
  const handle = normalizeXHandle(username);
  if (!handle) return;
  try {
    await withLock(
      handle,
      () => refresh(handle),
      async () => null,
    );
  } catch {
    // Background refresh; the next request can retry.
  }
}
