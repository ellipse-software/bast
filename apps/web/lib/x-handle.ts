const HANDLE = /^[A-Za-z0-9_]{1,15}$/;
const RESERVED = new Set([
  "home",
  "explore",
  "search",
  "settings",
  "i",
  "intent",
  "share",
  "compose",
  "messages",
  "notifications",
  "login",
  "signup",
  "tos",
  "privacy",
]);

const PROFILE_URL =
  /^https?:\/\/(?:(?:www|mobile)\.)?(?:x|twitter)\.com\/@?([A-Za-z0-9_]{1,15})\/?(?:\?.*)?$/i;

/** Strip @, trim, and lowercase. Returns null if it is not a valid X handle. */
export function normalizeXHandle(input: string): string | null {
  const trimmed = input.trim().replace(/^@+/, "");
  if (!HANDLE.test(trimmed)) return null;
  return trimmed.toLowerCase();
}

/** Extract a handle from an x.com / twitter.com profile URL. */
export function parseXProfileUrl(input: string): string | null {
  const match = input.trim().match(PROFILE_URL);
  const handle = match?.[1];
  if (!handle || RESERVED.has(handle.toLowerCase())) return null;
  return normalizeXHandle(handle);
}
