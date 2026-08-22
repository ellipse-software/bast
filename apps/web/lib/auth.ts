import { createHash, randomBytes, randomInt } from "node:crypto";

import { legalVersion } from "@/lib/company";
import { sendVaultOTP } from "@/lib/email";
import { getRedis } from "@/lib/redis";

const OTP_TTL_SECONDS = 60 * 10;
const TOKEN_TTL_SECONDS = 60 * 60 * 24 * 90;
const OTP_MAX_ATTEMPTS = 5;
const OTP_SEND_COOLDOWN_SECONDS = 30;
const OTP_SEND_MAX_PER_HOUR = 10;

function normalizeEmail(email: string): string {
  return email.trim().toLowerCase();
}

function hashToken(token: string): string {
  return createHash("sha256").update(token).digest("hex");
}

/** Digits only. Exact 6 digits required after normalization (no left-pad). */
export function normalizeOTP(value: unknown): string {
  if (value == null) return "";
  const digits = String(value).replace(/\D/g, "");
  return digits.length === 6 ? digits : "";
}

type StoredOTP = {
  code: string;
  acceptTerms?: boolean;
  termsVersion?: string;
};

type VaultUserRecord = {
  id: string;
  email: string;
  acceptedTermsAt?: string;
  termsVersion?: string;
};

export async function startEmailOTP(
  emailRaw: string,
  clientIP = "",
  opts: { acceptTerms?: boolean } = {},
): Promise<void> {
  const email = normalizeEmail(emailRaw);
  if (!email.includes("@")) {
    throw Object.assign(new Error("invalid email"), { status: 400 });
  }
  const redis = getRedis();
  const cooldownKey = `vault:otp:send:${email}`;
  const hourKey = `vault:otp:sendhour:${email}`;
  const ipKey = clientIP ? `vault:otp:sendip:${clientIP}` : "";

  const cooled = await redis.get(cooldownKey);
  if (cooled) {
    throw Object.assign(new Error("wait before requesting another code"), { status: 429 });
  }
  const hourCount = Number((await redis.get<number | string>(hourKey)) ?? 0);
  if (hourCount >= OTP_SEND_MAX_PER_HOUR) {
    throw Object.assign(new Error("too many codes requested"), { status: 429 });
  }
  if (ipKey) {
    const ipCount = Number((await redis.get<number | string>(ipKey)) ?? 0);
    if (ipCount >= OTP_SEND_MAX_PER_HOUR) {
      throw Object.assign(new Error("too many codes requested"), { status: 429 });
    }
  }

  const otp = String(randomInt(0, 1_000_000)).padStart(6, "0");
  const payload: StoredOTP = { code: otp };
  if (opts.acceptTerms) {
    payload.acceptTerms = true;
    payload.termsVersion = legalVersion;
  }
  await redis.set(`vault:otp:${email}`, payload, { ex: OTP_TTL_SECONDS });
  await redis.del(`vault:otp:attempts:${email}`);
  await redis.set(cooldownKey, "1", { ex: OTP_SEND_COOLDOWN_SECONDS });
  if (hourCount <= 0) {
    await redis.set(hourKey, 1, { ex: 60 * 60 });
  } else {
    await redis.incr(hourKey);
  }
  if (ipKey) {
    const ipCount = Number((await redis.get<number | string>(ipKey)) ?? 0);
    if (ipCount <= 0) {
      await redis.set(ipKey, 1, { ex: 60 * 60 });
    } else {
      await redis.incr(ipKey);
    }
  }
  await sendVaultOTP(email, otp);
}

export type VerifiedDevice = {
  token: string;
  userId: string;
  email: string;
  deviceId: string;
};

export async function verifyEmailOTP(emailRaw: string, codeRaw: string): Promise<VerifiedDevice> {
  const email = normalizeEmail(emailRaw);
  const code = normalizeOTP(codeRaw);
  if (code.length !== 6) {
    throw Object.assign(new Error("invalid or expired code"), { status: 401 });
  }
  const redis = getRedis();
  const attemptsKey = `vault:otp:attempts:${email}`;
  const attempts = Number((await redis.get<number | string>(attemptsKey)) ?? 0);
  if (attempts >= OTP_MAX_ATTEMPTS) {
    await redis.del(`vault:otp:${email}`);
    throw Object.assign(new Error("invalid or expired code"), { status: 401 });
  }

  const raw = await redis.get<StoredOTP | string | number>(`vault:otp:${email}`);
  const storedOTP =
    raw && typeof raw === "object" && "code" in raw ? (raw as StoredOTP) : null;
  const expected = storedOTP
    ? normalizeOTP(storedOTP.code)
    : normalizeOTP(raw);
  if (!expected || expected.length !== 6 || expected !== code) {
    if (attempts <= 0) {
      await redis.set(attemptsKey, 1, { ex: OTP_TTL_SECONDS });
    } else {
      await redis.incr(attemptsKey);
    }
    throw Object.assign(new Error("invalid or expired code"), { status: 401 });
  }
  await redis.del(`vault:otp:${email}`);
  await redis.del(attemptsKey);

  let userId = await redis.get<string>(`vault:user:email:${email}`);
  if (!userId) {
    userId = randomBytes(16).toString("hex");
    await redis.set(`vault:user:email:${email}`, userId);
  }
  const userKey = `vault:user:${userId}`;
  const existingUser = await redis.get<string>(userKey);
  let user: VaultUserRecord = { id: userId, email };
  if (existingUser) {
    try {
      const parsed =
        typeof existingUser === "string"
          ? (JSON.parse(existingUser) as VaultUserRecord)
          : (existingUser as VaultUserRecord);
      if (parsed?.id && parsed?.email) {
        user = parsed;
      }
    } catch {
      // keep the default record
    }
  }
  if (storedOTP?.acceptTerms) {
    user.acceptedTermsAt = new Date().toISOString();
    user.termsVersion = storedOTP.termsVersion || legalVersion;
  }
  await redis.set(userKey, JSON.stringify(user));

  const deviceId = randomBytes(12).toString("hex");
  const token = randomBytes(32).toString("base64url");
  const tokenHash = hashToken(token);
  await redis.set(
    `vault:token:${tokenHash}`,
    JSON.stringify({ userId, email, deviceId }),
    { ex: TOKEN_TTL_SECONDS },
  );
  return { token, userId, email, deviceId };
}

export type DeviceSession = {
  userId: string;
  email: string;
  deviceId: string;
};

export async function resolveBearer(authorization: string | null): Promise<DeviceSession | null> {
  if (!authorization?.startsWith("Bearer ")) return null;
  const token = authorization.slice("Bearer ".length).trim();
  if (!token) return null;
  const redis = getRedis();
  const raw = await redis.get<string>(`vault:token:${hashToken(token)}`);
  if (!raw) return null;
  try {
    const parsed = typeof raw === "string" ? JSON.parse(raw) : raw;
    if (!parsed?.userId || !parsed?.email) return null;
    return { userId: parsed.userId, email: parsed.email, deviceId: parsed.deviceId || "" };
  } catch {
    return null;
  }
}

export async function revokeBearer(authorization: string | null): Promise<void> {
  if (!authorization?.startsWith("Bearer ")) return;
  const token = authorization.slice("Bearer ".length).trim();
  if (!token) return;
  await getRedis().del(`vault:token:${hashToken(token)}`);
}
