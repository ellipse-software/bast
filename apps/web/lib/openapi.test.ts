import { describe, expect, test } from "bun:test";

import { openApiOperationIds, openApiSpec } from "@/lib/openapi";

describe("openApiSpec", () => {
  test("is OpenAPI 3.1 with identity and a server", () => {
    expect(openApiSpec.openapi).toBe("3.1.0");
    expect(openApiSpec.info.title).toBe("Bast.sh HTTP API");
    expect(openApiSpec.servers[0]?.url).toBe("https://bast.sh");
  });

  test("gives every operation a unique operationId and description", () => {
    const ids = openApiOperationIds();
    expect(ids.length).toBeGreaterThan(8);
    expect(new Set(ids).size).toBe(ids.length);

    for (const pathItem of Object.values(openApiSpec.paths)) {
      for (const operation of Object.values(pathItem)) {
        expect(operation.operationId.length).toBeGreaterThan(0);
        expect(operation.description.length).toBeGreaterThan(20);
        expect(Object.keys(operation.responses).length).toBeGreaterThan(0);
      }
    }
  });

  test("documents JSON error schemas on mutating and authenticated routes", () => {
    const vaultGet = openApiSpec.paths["/api/vault"].get;
    expect(vaultGet.responses["401"].content["application/json"].schema).toEqual(
      { $ref: "#/components/schemas/ApiError" },
    );
    expect(openApiSpec.components.schemas.ApiError.required).toEqual([
      "error",
      "code",
      "message",
      "hint",
    ]);
  });
});
