import { createHash, randomBytes } from "node:crypto";

import { getVaultObject, putVaultObject } from "@/lib/r2";
import { getRedis } from "@/lib/redis";

const MAX_VAULT_BYTES = 1 << 20;

export type VaultMeta = {
  revision: string;
  r2Key: string;
  size: number;
  updatedAt: number;
  contentHash: string;
};

function metaKey(userId: string): string {
  return `vault:meta:${userId}`;
}

function objectKey(userId: string, revision: string): string {
  return `vaults/${userId}/${revision}.json`;
}

export async function getVaultMeta(userId: string): Promise<VaultMeta | null> {
  const raw = await getRedis().get<string>(metaKey(userId));
  if (!raw) return null;
  return typeof raw === "string" ? (JSON.parse(raw) as VaultMeta) : (raw as VaultMeta);
}

export async function readVault(userId: string): Promise<{ meta: VaultMeta; body: Uint8Array } | null> {
  const meta = await getVaultMeta(userId);
  if (!meta) return null;
  const body = await getVaultObject(meta.r2Key);
  if (!body) return null;
  return { meta, body };
}

export async function writeVault(
  userId: string,
  body: Uint8Array,
  ifMatch: string | null,
): Promise<VaultMeta> {
  if (body.byteLength > MAX_VAULT_BYTES) {
    throw new Error("vault payload exceeds 1 MiB limit");
  }
  const current = await getVaultMeta(userId);
  if (ifMatch) {
    if (!current || current.revision !== ifMatch) {
      const err = new Error("precondition failed");
      (err as Error & { status: number }).status = 412;
      throw err;
    }
  }
  const revision = randomBytes(16).toString("hex");
  const r2Key = objectKey(userId, revision);
  const contentHash = createHash("sha256").update(body).digest("hex");
  await putVaultObject(r2Key, body);
  const meta: VaultMeta = {
    revision,
    r2Key,
    size: body.byteLength,
    updatedAt: Math.floor(Date.now() / 1000),
    contentHash,
  };
  await getRedis().set(metaKey(userId), JSON.stringify(meta));
  return meta;
}
