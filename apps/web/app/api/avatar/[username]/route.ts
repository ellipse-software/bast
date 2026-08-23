import { after } from "next/server";

import { jsonError } from "@/lib/api-error";
import {
  avatarIsStale,
  getAvatar,
  revalidateAvatar,
  type AvatarImage,
} from "@/lib/avatar";
import { normalizeXHandle } from "@/lib/x-handle";

export const runtime = "nodejs";

function cacheHeaders(etag: string): HeadersInit {
  return {
    "Cache-Control":
      "public, max-age=86400, s-maxage=604800, stale-while-revalidate=86400",
    ETag: `"${etag}"`,
  };
}

function imageResponse(image: AvatarImage): Response {
  const headers = new Headers(cacheHeaders(image.etag));
  headers.set("Content-Type", image.contentType);
  return new Response(Buffer.from(image.body), { status: 200, headers });
}

export async function GET(
  request: Request,
  context: { params: Promise<{ username: string }> },
) {
  const { username } = await context.params;
  const handle = normalizeXHandle(username);
  if (!handle) {
    return jsonError(400, {
      code: "invalid_username",
      message: "Username is not a valid X handle.",
      hint: "Use a 1–15 character alphanumeric or underscore handle, without @.",
    });
  }

  const avatar = await getAvatar(handle);
  if (avatar === "missing" || avatar === null) {
    return new Response(null, {
      status: 404,
      headers: {
        "Cache-Control": "public, max-age=300, s-maxage=1800",
      },
    });
  }

  const ifNoneMatch = request.headers.get("if-none-match")?.replaceAll('"', "");
  if (ifNoneMatch && ifNoneMatch === avatar.etag) {
    return new Response(null, {
      status: 304,
      headers: cacheHeaders(avatar.etag),
    });
  }

  if (await avatarIsStale(handle)) {
    after(() => {
      void revalidateAvatar(handle);
    });
  }

  return imageResponse(avatar);
}
