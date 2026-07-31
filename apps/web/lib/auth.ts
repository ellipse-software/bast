import { betterAuth } from "better-auth";
import { memoryAdapter } from "better-auth/adapters/memory";
import { emailOTP } from "better-auth/plugins";
import { createHash, randomBytes, randomInt } from "node:crypto";

import { sendVaultOTP } from "@/lib/email";
import { getRedis, redisConfigured } from "@/lib/redis";

const OTP_TTL_SECONDS = 60 * 10;
const TOKEN_TTL_SECONDS = 60 * 60 * 24 * 90;

async function sendOTPEmail(email: string, otp: string): Promise<void> {
  await sendVaultOTP(email, otp);
}

function secondaryStorage() {
  return {
    get: async (key: string) => {
      const value = await getRedis().get<string>(`ba:${key}`);
      return value ?? null;
    },
    set: async (key: string, value: string, ttl?: number) => {
      if (ttl) {
        await getRedis().set(`ba:${key}`, value, { ex: ttl });
      } else {
        await getRedis().set(`ba:${key}`, value);
      }
    },
    delete: async (key: string) => {
      await getRedis().del(`ba:${key}`);
    },
  };
}

/**
 * Better Auth with email OTP. Primary tables use the memory adapter; OTP and
 * rate-limit state go through Redis secondary storage when configured.
 * CLI bearer tokens are issued from Redis after OTP verification.
 */
export const auth = betterAuth({
  secret: process.env.BETTER_AUTH_SECRET || "dev-only-change-me-bast-vault-secret",
  baseURL: process.env.BETTER_AUTH_URL || process.env.NEXT_PUBLIC_SITE_URL || "https://bast.sh",
  database: memoryAdapter({}),
  emailAndPassword: { enabled: false },
  ...(typeof process !== "undefined" && redisConfigured()
    ? { secondaryStorage: secondaryStorage() }
    : {}),
  plugins: [
    emailOTP({
      async sendVerificationOTP({ email, otp }) {
        await sendOTPEmail(email, otp);
      },
      otpLength: 6,
      expiresIn: OTP_TTL_SECONDS,
    }),
  ],
});

function normalizeEmail(email: string): string {
  return email.trim().toLowerCase();
}

function hashToken(token: string): string {
  return createHash("sha256").update(token).digest("hex");
}

/** Digits only, left-padded to 6. Upstash JSON-parses bare numeric strings into numbers. */
function normalizeOTP(value: unknown): string {
  if (value == null) return "";
  const digits = String(value).replace(/\D/g, "");
  if (!digits) return "";
  return digits.padStart(6, "0");
}

type StoredOTP = { code: string };

export async function startEmailOTP(emailRaw: string): Promise<void> {
  const email = normalizeEmail(emailRaw);
  if (!email.includes("@")) {
    throw new Error("invalid email");
  }
  const otp = String(randomInt(0, 1_000_000)).padStart(6, "0");
  const redis = getRedis();
  // Store as an object so Upstash does not coerce "123456" into a number on GET.
  const payload: StoredOTP = { code: otp };
  await redis.set(`vault:otp:${email}`, payload, { ex: OTP_TTL_SECONDS });
  await sendOTPEmail(email, otp);
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
  const redis = getRedis();
  const raw = await redis.get<StoredOTP | string | number>(`vault:otp:${email}`);
  const expected =
    raw && typeof raw === "object" && "code" in raw
      ? normalizeOTP((raw as StoredOTP).code)
      : normalizeOTP(raw);
  if (!expected || !code || expected !== code) {
    throw new Error("invalid or expired code");
  }
  await redis.del(`vault:otp:${email}`);

  let userId = await redis.get<string>(`vault:user:email:${email}`);
  if (!userId) {
    userId = randomBytes(16).toString("hex");
    await redis.set(`vault:user:email:${email}`, userId);
    await redis.set(`vault:user:${userId}`, JSON.stringify({ id: userId, email }));
  }

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
  const parsed = typeof raw === "string" ? JSON.parse(raw) : raw;
  if (!parsed?.userId || !parsed?.email) return null;
  return { userId: parsed.userId, email: parsed.email, deviceId: parsed.deviceId || "" };
}

export async function revokeBearer(authorization: string | null): Promise<void> {
  if (!authorization?.startsWith("Bearer ")) return;
  const token = authorization.slice("Bearer ".length).trim();
  if (!token) return;
  await getRedis().del(`vault:token:${hashToken(token)}`);
}
