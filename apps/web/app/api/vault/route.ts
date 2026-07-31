import { resolveBearer } from "@/lib/auth";
import { r2Configured } from "@/lib/r2";
import { redisConfigured } from "@/lib/redis";
import { getVaultMeta, readVault, writeVault } from "@/lib/vault-store";

function unauthorized() {
  return Response.json({ error: "unauthorized" }, { status: 401 });
}

function stripETag(value: string | null): string | null {
  if (!value) return null;
  return value.trim().replaceAll(`"`, "");
}

export async function GET(request: Request) {
  if (!redisConfigured() || !r2Configured()) {
    return Response.json({ error: "vault storage is not configured" }, { status: 503 });
  }
  const session = await resolveBearer(request.headers.get("authorization"));
  if (!session) return unauthorized();

  const ifNoneMatch = stripETag(request.headers.get("if-none-match"));
  const meta = await getVaultMeta(session.userId);
  if (!meta) {
    return new Response(null, { status: 404 });
  }
  if (ifNoneMatch && ifNoneMatch === meta.revision) {
    return new Response(null, {
      status: 304,
      headers: {
        ETag: `"${meta.revision}"`,
        "X-Vault-Updated-At": String(meta.updatedAt),
      },
    });
  }
  const vault = await readVault(session.userId);
  if (!vault) {
    return new Response(null, { status: 404 });
  }
  return new Response(Buffer.from(vault.body), {
    status: 200,
    headers: {
      "Content-Type": "application/json",
      ETag: `"${vault.meta.revision}"`,
      "X-Vault-Updated-At": String(vault.meta.updatedAt),
      "Cache-Control": "no-store",
    },
  });
}

export async function PUT(request: Request) {
  if (!redisConfigured() || !r2Configured()) {
    return Response.json({ error: "vault storage is not configured" }, { status: 503 });
  }
  const session = await resolveBearer(request.headers.get("authorization"));
  if (!session) return unauthorized();

  const ifMatch = stripETag(request.headers.get("if-match"));
  const buf = new Uint8Array(await request.arrayBuffer());
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
        headers: {
          ETag: `"${meta.revision}"`,
          "X-Vault-Updated-At": String(meta.updatedAt),
        },
      },
    );
  } catch (error) {
    const status = error && typeof error === "object" && "status" in error ? Number(error.status) : 400;
    const message = error instanceof Error ? error.message : "write failed";
    return Response.json({ error: message }, { status: status || 400 });
  }
}
