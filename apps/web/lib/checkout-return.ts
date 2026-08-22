import { siteUrl } from "@/lib/site";

const LOCAL_HOSTS = new Set(["localhost", "127.0.0.1", "192.168.0.232"]);

export const SPONSOR_COMPLETE_PARAM = "sponsor";
export const SPONSOR_COMPLETE_VALUE = "complete";
export const SPONSOR_OPEN_VALUE = "open";
export const SPONSOR_SESSION_PARAM = "session_id";
export const SPONSOR_OPEN_HREF = `/?${SPONSOR_COMPLETE_PARAM}=${SPONSOR_OPEN_VALUE}`;

export function isCheckoutSessionId(value: string): boolean {
  return /^cs_(test|live)_[A-Za-z0-9]+$/.test(value);
}

export function sanitizeCheckoutReturnPath(path: unknown): string {
  if (typeof path !== "string") return "/";
  if (!path.startsWith("/") || path.startsWith("//") || path.includes("\\")) {
    return "/";
  }
  const clean = path.split("?")[0]?.split("#")[0] ?? "/";
  if (clean.length === 0 || clean.length > 200) return "/";
  if (clean.includes("://") || clean.includes("%")) return "/";
  return clean;
}

export function checkoutOrigin(input: {
  vercelEnv: string | undefined;
  vercelUrl: string | undefined;
  requestOrigin: string | null;
}): string {
  if (input.vercelEnv === "production") {
    return siteUrl;
  }
  if (input.vercelUrl) {
    return `https://${input.vercelUrl}`;
  }
  if (input.requestOrigin) {
    try {
      const url = new URL(input.requestOrigin);
      if (LOCAL_HOSTS.has(url.hostname)) {
        return url.origin;
      }
    } catch {
      // Fall through to the public site origin.
    }
  }
  return siteUrl;
}

export function requestOriginFromHeaders(input: {
  forwardedHost: string | null;
  host: string | null;
  forwardedProto: string | null;
}): string | null {
  const host =
    input.forwardedHost?.split(",")[0]?.trim() || input.host?.trim() || "";
  if (!host) return null;
  const proto = input.forwardedProto?.split(",")[0]?.trim() || "http";
  return `${proto}://${host}`;
}

export function parseSponsorDialogIntent(search: string): {
  open: boolean;
  sessionId: string | null;
} {
  const params = new URLSearchParams(
    search.startsWith("?") ? search.slice(1) : search,
  );
  const sponsor = params.get(SPONSOR_COMPLETE_PARAM);
  if (sponsor === SPONSOR_COMPLETE_VALUE) {
    const rawId = params.get(SPONSOR_SESSION_PARAM);
    return {
      open: true,
      sessionId: rawId && isCheckoutSessionId(rawId) ? rawId : null,
    };
  }
  if (sponsor === SPONSOR_OPEN_VALUE) {
    return { open: true, sessionId: null };
  }
  return { open: false, sessionId: null };
}

export function stripSponsorDialogSearch(search: string): string {
  const params = new URLSearchParams(
    search.startsWith("?") ? search.slice(1) : search,
  );
  const sponsor = params.get(SPONSOR_COMPLETE_PARAM);
  if (sponsor !== SPONSOR_COMPLETE_VALUE && sponsor !== SPONSOR_OPEN_VALUE) {
    return params.toString();
  }
  params.delete(SPONSOR_COMPLETE_PARAM);
  params.delete(SPONSOR_SESSION_PARAM);
  return params.toString();
}

/** Stripe replaces `{CHECKOUT_SESSION_ID}` literally; do not URL-encode it. */
export function buildCheckoutReturnUrl(origin: string, path: string): string {
  const safeOrigin = origin.replace(/\/$/, "");
  const safePath = sanitizeCheckoutReturnPath(path);
  return `${safeOrigin}${safePath}?${SPONSOR_COMPLETE_PARAM}=${SPONSOR_COMPLETE_VALUE}&${SPONSOR_SESSION_PARAM}={CHECKOUT_SESSION_ID}`;
}
