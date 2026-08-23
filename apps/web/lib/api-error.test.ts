import { describe, expect, test } from "bun:test";

import { apiErrorPayload, jsonError } from "@/lib/api-error";

describe("apiErrorPayload", () => {
  test("keeps a string error field for existing CLI clients", () => {
    const payload = apiErrorPayload({
      code: "unauthorized",
      message: "A valid Bearer token is required.",
      hint: "Send Authorization: Bearer <token>.",
    });
    expect(payload.error).toBe(payload.message);
    expect(payload.code).toBe("unauthorized");
    expect(payload.hint).toContain("Bearer");
  });
});

describe("jsonError", () => {
  test("returns JSON with the given status", async () => {
    const response = jsonError(404, {
      code: "not_found",
      message: "This API route does not exist.",
      hint: "See https://bast.sh/openapi.json.",
    });
    expect(response.status).toBe(404);
    expect(response.headers.get("content-type")).toContain("application/json");
    const body = (await response.json()) as {
      error: string;
      code: string;
      message: string;
      hint: string;
    };
    expect(body.code).toBe("not_found");
    expect(body.error).toBe(body.message);
    expect(body.hint).toContain("openapi.json");
  });
});
