import { getRedis, redisConfigured } from "@/lib/redis";
import { normalizeXHandle } from "@/lib/x-handle";

export type XProfile = {
  name: string;
  avatarUrl: string | null;
  verified: boolean;
};

type CachedProfile = {
  status: "ok" | "missing";
  name?: string;
  avatarUrl?: string | null;
  verified?: boolean;
  fetchedAt: number;
};

const META_PREFIX = "xprofile:v1:";
const LOCK_PREFIX = "xprofile:lock:v1:";

const FRESH_MS = 7 * 24 * 60 * 60 * 1000;
const MISSING_MS = 6 * 60 * 60 * 1000;
const OK_TTL_SEC = 30 * 24 * 60 * 60;
const MISSING_TTL_SEC = 6 * 60 * 60;
const LOCK_SEC = 20;
const FETCH_MS = 4_000;

export function upgradeTwitterAvatarUrl(url: string): string {
  return url.replace(
    /_(?:normal|bigger|mini|200x200)(\.[a-z0-9]+)$/i,
    "_400x400$1",
  );
}

export function parseFxProfile(payload: unknown): XProfile | null {
  if (!payload || typeof payload !== "object") return null;
  const user = (payload as { user?: unknown }).user;
  if (!user || typeof user !== "object") return null;

  const record = user as {
    name?: unknown;
    avatar_url?: unknown;
    verification?: unknown;
  };
  const name = typeof record.name === "string" ? record.name.trim() : "";
  if (!name) return null;

  const avatarUrl =
    typeof record.avatar_url === "string" && record.avatar_url.length > 0
      ? upgradeTwitterAvatarUrl(record.avatar_url)
      : null;

  const verification = record.verification;
  const verified =
    Boolean(
      verification &&
        typeof verification === "object" &&
        (verification as { verified?: unknown }).verified === true,
    );

  return { name, avatarUrl, verified };
}

function metaKey(handle: string): string {
  return `${META_PREFIX}${handle}`;
}

function lockKey(handle: string): string {
  return `${LOCK_PREFIX}${handle}`;
}

function isFresh(meta: CachedProfile): boolean {
  const age = Date.now() - meta.fetchedAt;
  if (meta.status === "missing") return age < MISSING_MS;
  return age < FRESH_MS;
}

function asProfile(meta: CachedProfile): XProfile | null {
  if (meta.status !== "ok" || !meta.name) return null;
  return {
    name: meta.name,
    avatarUrl: meta.avatarUrl ?? null,
    verified: Boolean(meta.verified),
  };
}

async function readMeta(handle: string): Promise<CachedProfile | null> {
  if (!redisConfigured()) return null;
  const raw = await getRedis().get<CachedProfile | string>(metaKey(handle));
  if (!raw) return null;
  try {
    return typeof raw === "string" ? (JSON.parse(raw) as CachedProfile) : raw;
  } catch {
    return null;
  }
}

async function writeMeta(handle: string, meta: CachedProfile): Promise<void> {
  if (!redisConfigured()) return;
  const ttl = meta.status === "missing" ? MISSING_TTL_SEC : OK_TTL_SEC;
  await getRedis().set(metaKey(handle), meta, { ex: ttl });
}

export async function rememberXProfile(
  username: string,
  profile: XProfile,
): Promise<void> {
  const handle = normalizeXHandle(username);
  if (!handle || !profile.name.trim()) return;
  await writeMeta(handle, {
    status: "ok",
    name: profile.name,
    avatarUrl: profile.avatarUrl,
    verified: profile.verified,
    fetchedAt: Date.now(),
  });
}

async function fetchFxProfile(handle: string): Promise<XProfile | null> {
  const init: RequestInit & { next?: { revalidate: number } } = {
    headers: { Accept: "application/json", "User-Agent": "bast.sh" },
    signal: AbortSignal.timeout(FETCH_MS),
    next: { revalidate: 3600 },
  };
  const response = await fetch(
    `https://api.fxtwitter.com/2/profile/${handle}`,
    init,
  );
  if (!response.ok) return null;
  return parseFxProfile(await response.json());
}

async function refresh(handle: string): Promise<XProfile | null> {
  const profile = await fetchFxProfile(handle);
  if (!profile) {
    await writeMeta(handle, { status: "missing", fetchedAt: Date.now() });
    return null;
  }
  await writeMeta(handle, {
    status: "ok",
    name: profile.name,
    avatarUrl: profile.avatarUrl,
    verified: profile.verified,
    fetchedAt: Date.now(),
  });
  return profile;
}

async function withLock(
  handle: string,
  task: () => Promise<XProfile | null>,
): Promise<XProfile | null> {
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
  const meta = await readMeta(handle);
  if (meta) {
    if (meta.status === "missing" && isFresh(meta)) return null;
    const profile = asProfile(meta);
    if (profile) return profile;
  }
  return task();
}

export async function getXProfile(username: string): Promise<XProfile | null> {
  const handle = normalizeXHandle(username);
  if (!handle) return null;

  try {
    const meta = await readMeta(handle);
    if (meta?.status === "missing" && isFresh(meta)) return null;
    if (meta && isFresh(meta)) {
      const profile = asProfile(meta);
      if (profile) return profile;
    }

    return await withLock(handle, () => refresh(handle));
  } catch {
    try {
      return await fetchFxProfile(handle);
    } catch {
      return null;
    }
  }
}

export async function getXProfiles(
  usernames: readonly string[],
): Promise<Map<string, XProfile>> {
  const unique = [
    ...new Set(
      usernames
        .map((username) => normalizeXHandle(username))
        .filter((handle): handle is string => Boolean(handle)),
    ),
  ];

  const resolved = await Promise.all(
    unique.map(async (handle) => {
      const profile = await getXProfile(handle);
      return profile ? ([handle, profile] as const) : null;
    }),
  );

  return new Map(resolved.filter((entry) => entry !== null));
}
