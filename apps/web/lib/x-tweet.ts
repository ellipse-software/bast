import { getRedis, redisConfigured } from "@/lib/redis";
import { normalizeXHandle } from "@/lib/x-handle";
import {
  parseFxProfile,
  rememberXProfile,
  type XProfile,
} from "@/lib/x-profile";

export type XTweet = {
  id: string;
  url: string;
  text: string;
  username: string;
  profile: XProfile;
};

type CachedTweet = {
  status: "ok" | "missing";
  id?: string;
  url?: string;
  text?: string;
  username?: string;
  profile?: XProfile;
  fetchedAt: number;
};

const META_PREFIX = "xtweet:v1:";
const LOCK_PREFIX = "xtweet:lock:v1:";

const FRESH_MS = 7 * 24 * 60 * 60 * 1000;
const MISSING_MS = 6 * 60 * 60 * 1000;
const OK_TTL_SEC = 30 * 24 * 60 * 60;
const MISSING_TTL_SEC = 6 * 60 * 60;
const LOCK_SEC = 20;
const FETCH_MS = 4_000;

const TWEET_URL =
  /^https?:\/\/(?:(?:www|mobile)\.)?(?:x|twitter)\.com\/(?:[A-Za-z0-9_]+\/status|i\/web\/status)\/(\d+)/i;

export function parseTweetUrl(
  input: string,
): { id: string; handle: string | null } | null {
  const trimmed = input.trim();
  const match = trimmed.match(TWEET_URL);
  if (!match?.[1]) return null;

  const handleMatch = trimmed.match(
    /(?:x|twitter)\.com\/([A-Za-z0-9_]+)\/status\//i,
  );
  const rawHandle = handleMatch?.[1];
  const handle =
    rawHandle && rawHandle.toLowerCase() !== "i"
      ? normalizeXHandle(rawHandle)
      : null;

  return { id: match[1], handle };
}

export function parseFxStatus(payload: unknown): XTweet | null {
  if (!payload || typeof payload !== "object") return null;
  const record = payload as { status?: unknown; tweet?: unknown };
  const status = record.status ?? record.tweet;
  if (!status || typeof status !== "object") return null;

  const tweet = status as {
    id?: unknown;
    url?: unknown;
    text?: unknown;
    author?: unknown;
  };

  const id = typeof tweet.id === "string" ? tweet.id : "";
  if (!/^\d{1,20}$/.test(id)) return null;

  const text = typeof tweet.text === "string" ? tweet.text.trim() : "";
  if (!text) return null;

  const profile = parseFxProfile({ user: tweet.author });
  if (!profile) return null;

  const author = tweet.author as { screen_name?: unknown };
  const screenName =
    typeof author.screen_name === "string"
      ? author.screen_name.replace(/^@+/, "")
      : "";
  if (!normalizeXHandle(screenName)) return null;

  const url =
    typeof tweet.url === "string" && tweet.url.startsWith("https://")
      ? tweet.url
      : `https://x.com/${screenName}/status/${id}`;

  return { id, url, text, username: screenName, profile };
}

function metaKey(id: string): string {
  return `${META_PREFIX}${id}`;
}

function lockKey(id: string): string {
  return `${LOCK_PREFIX}${id}`;
}

function isFresh(meta: CachedTweet): boolean {
  const age = Date.now() - meta.fetchedAt;
  if (meta.status === "missing") return age < MISSING_MS;
  return age < FRESH_MS;
}

function asTweet(meta: CachedTweet): XTweet | null {
  if (
    meta.status !== "ok" ||
    !meta.id ||
    !meta.url ||
    !meta.text ||
    !meta.username ||
    !meta.profile
  ) {
    return null;
  }
  return {
    id: meta.id,
    url: meta.url,
    text: meta.text,
    username: meta.username,
    profile: meta.profile,
  };
}

async function readMeta(id: string): Promise<CachedTweet | null> {
  if (!redisConfigured()) return null;
  const raw = await getRedis().get<CachedTweet | string>(metaKey(id));
  if (!raw) return null;
  try {
    return typeof raw === "string" ? (JSON.parse(raw) as CachedTweet) : raw;
  } catch {
    return null;
  }
}

async function writeMeta(id: string, meta: CachedTweet): Promise<void> {
  if (!redisConfigured()) return;
  const ttl = meta.status === "missing" ? MISSING_TTL_SEC : OK_TTL_SEC;
  await getRedis().set(metaKey(id), meta, { ex: ttl });
}

async function fetchFxStatus(id: string): Promise<XTweet | null> {
  const init: RequestInit & { next?: { revalidate: number } } = {
    headers: { Accept: "application/json", "User-Agent": "bast.sh" },
    signal: AbortSignal.timeout(FETCH_MS),
    next: { revalidate: 3600 },
  };
  const response = await fetch(`https://api.fxtwitter.com/2/status/${id}`, init);
  if (!response.ok) return null;
  return parseFxStatus(await response.json());
}

async function refresh(id: string): Promise<XTweet | null> {
  const tweet = await fetchFxStatus(id);
  if (!tweet) {
    await writeMeta(id, { status: "missing", fetchedAt: Date.now() });
    return null;
  }
  await writeMeta(id, {
    status: "ok",
    id: tweet.id,
    url: tweet.url,
    text: tweet.text,
    username: tweet.username,
    profile: tweet.profile,
    fetchedAt: Date.now(),
  });
  await rememberXProfile(tweet.username, tweet.profile);
  return tweet;
}

async function withLock(
  id: string,
  task: () => Promise<XTweet | null>,
): Promise<XTweet | null> {
  if (!redisConfigured()) return task();

  const acquired = await getRedis().set(lockKey(id), 1, {
    nx: true,
    ex: LOCK_SEC,
  });
  if (acquired) {
    try {
      return await task();
    } finally {
      await getRedis().del(lockKey(id));
    }
  }

  await new Promise((resolve) => setTimeout(resolve, 400));
  const meta = await readMeta(id);
  if (meta) {
    if (meta.status === "missing" && isFresh(meta)) return null;
    const tweet = asTweet(meta);
    if (tweet) return tweet;
  }
  return task();
}

export async function getXTweet(url: string): Promise<XTweet | null> {
  const parsed = parseTweetUrl(url);
  if (!parsed) return null;
  const { id } = parsed;

  try {
    const meta = await readMeta(id);
    if (meta?.status === "missing" && isFresh(meta)) return null;
    if (meta && isFresh(meta)) {
      const tweet = asTweet(meta);
      if (tweet) return tweet;
    }
    return await withLock(id, () => refresh(id));
  } catch {
    try {
      return await fetchFxStatus(id);
    } catch {
      return null;
    }
  }
}

export async function getXTweets(
  urls: readonly string[],
): Promise<XTweet[]> {
  const resolved = await Promise.all(urls.map((url) => getXTweet(url)));
  return resolved.filter((tweet): tweet is XTweet => tweet !== null);
}
