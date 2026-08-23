import { jsonError } from "@/lib/api-error";
import { resolveBearer } from "@/lib/auth";
import { r2Configured } from "@/lib/r2";
import { redisConfigured } from "@/lib/redis";
import { getVaultMeta, MAX_VAULT_BYTES, readVault, writeVault } from "@/lib/vault-store";

function unauthorized() {
  return jsonError(401, {
    code: "unauthorized",
    message: "A valid Bearer token is required.",
    hint: "POST /api/auth/otp/start, then POST /api/auth/otp/verify, and send Authorization: Bearer <token>.",
  });
}

function vaultUnconfigured() {
  return jsonError(503, {
    code: "vault_unconfigured",
    message: "Vault storage is not configured on this origin.",
    hint: "Self-host with Upstash Redis and Cloudflare R2, or use https://bast.sh. See https://bast.sh/docs/reference/self-hosting.",
  });
}

function stripETag(value: string | null): string | null {
  if (!value) return null;
  return value.trim().replaceAll(`"`, "");
}

function noStoreHeaders(extra: Record<string, string> = {}) {
  return { "Cache-Control": "no-store", ...extra };
}

export async function GET(request: Request) {
  if (!redisConfigured() || !r2Configured()) {
    return vaultUnconfigured();
  }
  const session = await resolveBearer(request.headers.get("authorization"));
  if (!session) return unauthorized();

  try {
    const ifNoneMatch = stripETag(request.headers.get("if-none-match"));
    const meta = await getVaultMeta(session.userId);
    if (!meta) {
      return jsonError(
        404,
        {
          code: "vault_not_found",
          message: "No vault exists for this account yet.",
          hint: "PUT /api/vault with Authorization and no If-Match to create the first revision.",
        },
        noStoreHeaders(),
      );
    }
    if (ifNoneMatch && ifNoneMatch === meta.revision) {
      return new Response(null, {
        status: 304,
        headers: noStoreHeaders({
          ETag: `"${meta.revision}"`,
          "X-Vault-Updated-At": String(meta.updatedAt),
        }),
      });
    }
    const vault = await readVault(session.userId);
    if (!vault) {
      return jsonError(
        404,
        {
          code: "vault_not_found",
          message: "No vault exists for this account yet.",
          hint: "PUT /api/vault with Authorization and no If-Match to create the first revision.",
        },
        noStoreHeaders(),
      );
    }
    return new Response(Buffer.from(vault.body), {
      status: 200,
      headers: noStoreHeaders({
        "Content-Type": "application/json",
        ETag: `"${vault.meta.revision}"`,
        "X-Vault-Updated-At": String(vault.meta.updatedAt),
      }),
    });
  } catch (error) {
    console.error("vault read failed", error);
    return jsonError(500, {
      code: "vault_read_failed",
      message: "The vault could not be read.",
      hint: "Retry the GET. If it persists, check https://bast.sh/status and https://bast.sh/api/health/vault.",
    });
  }
}

export async function PUT(request: Request) {
  if (!redisConfigured() || !r2Configured()) {
    return vaultUnconfigured();
  }
  const session = await resolveBearer(request.headers.get("authorization"));
  if (!session) return unauthorized();

  const tooLarge = {
    code: "vault_too_large",
    message: "Vault payload exceeds the 1 MiB limit.",
    hint: "Shrink the vault blob. Bast Vault rejects bodies larger than 1048576 bytes.",
  };

  const contentLength = Number(request.headers.get("content-length") || "0");
  if (contentLength > MAX_VAULT_BYTES) {
    return jsonError(400, tooLarge);
  }

  const ifMatch = stripETag(request.headers.get("if-match"));
  const buf = new Uint8Array(await request.arrayBuffer());
  if (buf.byteLength > MAX_VAULT_BYTES) {
    return jsonError(400, tooLarge);
  }
  try {
    const meta = await writeVault(session.userId, buf, ifMatch);
    return Response.json(
      {
        revision: meta.revision,
        updatedAt: meta.updatedAt,
        size: meta.size,
        contentHash: meta.contentHash,
      },
      {
        status: 200,
        headers: noStoreHeaders({
          ETag: `"${meta.revision}"`,
          "X-Vault-Updated-At": String(meta.updatedAt),
        }),
      },
    );
  } catch (error) {
    const known = error && typeof error === "object" && "status" in error ? Number(error.status) : 0;
    if (known >= 400 && known < 500) {
      const message = error instanceof Error ? error.message : "write failed";
      return jsonError(known, {
        code: known === 412 ? "precondition_failed" : "vault_write_rejected",
        message,
        hint:
          known === 412
            ? "GET /api/vault, merge, and PUT again with the current If-Match revision."
            : "Check the payload size and If-Match header. See https://bast.sh/openapi.json.",
      });
    }
    console.error("vault write failed", error);
    return jsonError(500, {
      code: "vault_write_failed",
      message: "The vault could not be written.",
      hint: "Retry the PUT. If it persists, check https://bast.sh/status and https://bast.sh/api/health/vault.",
    });
  }
}
