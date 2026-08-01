import { Files, FilesError } from "files-sdk";
import { r2 } from "files-sdk/r2";

let files: Files | null = null;

function getFiles(): Files {
  if (files) return files;
  const accountId = process.env.R2_ACCOUNT_ID;
  const bucket = process.env.R2_BUCKET_NAME;
  const accessKeyId = process.env.R2_ACCESS_KEY_ID;
  const secretAccessKey = process.env.R2_SECRET_ACCESS_KEY;
  if (!accountId || !bucket || !accessKeyId || !secretAccessKey) {
    throw new Error("R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, and R2_BUCKET_NAME are required");
  }
  // Lightweight SigV4 fetch client. No @aws-sdk/* needed for vault-sized blobs.
  files = new Files({
    adapter: r2({
      bucket,
      accountId,
      accessKeyId,
      secretAccessKey,
      client: "fetch",
    }),
  });
  return files;
}

export function r2Configured(): boolean {
  return Boolean(
    process.env.R2_ACCOUNT_ID &&
      process.env.R2_ACCESS_KEY_ID &&
      process.env.R2_SECRET_ACCESS_KEY &&
      process.env.R2_BUCKET_NAME,
  );
}

export async function putVaultObject(key: string, body: Uint8Array | Buffer | string): Promise<void> {
  await getFiles().upload(key, body, { contentType: "application/json" });
}

export async function deleteVaultObject(key: string): Promise<void> {
  try {
    await getFiles().delete(key);
  } catch (error) {
    if (error instanceof FilesError && error.code === "NotFound") {
      return;
    }
    throw error;
  }
}

export async function getVaultObject(key: string): Promise<Uint8Array | null> {
  try {
    const file = await getFiles().download(key);
    return new Uint8Array(await file.arrayBuffer());
  } catch (error) {
    if (error instanceof FilesError && error.code === "NotFound") {
      return null;
    }
    throw error;
  }
}
