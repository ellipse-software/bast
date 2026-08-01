import { resolveBearer } from "@/lib/auth";
import { r2Configured } from "@/lib/r2";
import { redisConfigured } from "@/lib/redis";
import { getVaultMeta, MAX_VAULT_BYTES, readVault, writeVault } from "@/lib/vault-store";

function unauthorized() {
  return Response.json({ error: "unauthorized" }, { status: 401 });
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
    return Response.json({ error: "vault storage is not configured" }, { status: 503 });
  }
  const session = await resolveBearer(request.headers.get("authorization"));
  if (!session) return unauthorized();

  try {
    const ifNoneMatch = stripETag(request.headers.get("if-none-match"));
    const meta = await getVaultMeta(session.userId);
    if (!meta) {
      return new Response(null, { status: 404, headers: noStoreHeaders() });
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
      return new Response(null, { status: 404, headers: noStoreHeaders() });
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
    return Response.json({ error: "read failed" }, { status: 500 });
  }
}

export async function PUT(request: Request) {
  if (!redisConfigured() || !r2Configured()) {
    return Response.json({ error: "vault storage is not configured" }, { status: 503 });
  }
  const session = await resolveBearer(request.headers.get("authorization"));
  if (!session) return unauthorized();

  const contentLength = Number(request.headers.get("content-length") || "0");
  if (contentLength > MAX_VAULT_BYTES) {
    return Response.json({ error: "vault payload exceeds 1 MiB limit" }, { status: 400 });
  }

  const ifMatch = stripETag(request.headers.get("if-match"));
  const buf = new Uint8Array(await request.arrayBuffer());
  if (buf.byteLength > MAX_VAULT_BYTES) {
    return Response.json({ error: "vault payload exceeds 1 MiB limit" }, { status: 400 });
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
      return Response.json({ error: message }, { status: known });
    }
    console.error("vault write failed", error);
    return Response.json({ error: "write failed" }, { status: 500 });
  }
}
