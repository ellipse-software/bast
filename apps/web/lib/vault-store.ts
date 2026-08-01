import { createHash, randomBytes } from "node:crypto";

import { deleteVaultObject, getVaultObject, putVaultObject } from "@/lib/r2";
import { getRedis } from "@/lib/redis";

export const MAX_VAULT_BYTES = 1 << 20;

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

const META_CAS_SCRIPT = `
local cur = redis.call('GET', KEYS[1])
local expected = ARGV[1]
local newmeta = ARGV[2]
if expected == '' then
  if cur ~= false then return 0 end
else
  if cur == false then return 0 end
  local obj = cjson.decode(cur)
  if obj['revision'] ~= expected then return 0 end
end
redis.call('SET', KEYS[1], newmeta)
return 1
`;

export async function getVaultMeta(userId: string): Promise<VaultMeta | null> {
  const raw = await getRedis().get<string>(metaKey(userId));
  if (!raw) return null;
  try {
    return typeof raw === "string" ? (JSON.parse(raw) as VaultMeta) : (raw as VaultMeta);
  } catch {
    return null;
  }
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
    const err = new Error("vault payload exceeds 1 MiB limit");
    (err as Error & { status: number }).status = 400;
    throw err;
  }
  const current = await getVaultMeta(userId);
  const expected = ifMatch ?? "";
  if (ifMatch) {
    if (!current || current.revision !== ifMatch) {
      const err = new Error("precondition failed");
      (err as Error & { status: number }).status = 412;
      throw err;
    }
  } else if (current) {
    // Creating without If-Match while a vault exists is a conflict.
    const err = new Error("precondition failed");
    (err as Error & { status: number }).status = 412;
    throw err;
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

  const redis = getRedis();
  const cas = await redis.eval<[string, string], number>(
    META_CAS_SCRIPT,
    [metaKey(userId)],
    [expected, JSON.stringify(meta)],
  );
  if (cas !== 1) {
    await deleteVaultObject(r2Key).catch(() => undefined);
    const err = new Error("precondition failed");
    (err as Error & { status: number }).status = 412;
    throw err;
  }
  if (current?.r2Key && current.r2Key !== r2Key) {
    await deleteVaultObject(current.r2Key).catch(() => undefined);
  }
  return meta;
}
