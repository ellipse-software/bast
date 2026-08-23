import { apiErrorPayload } from "@/lib/api-error";
import { company } from "@/lib/company";
import { siteUrl } from "@/lib/site";

const errorExample = apiErrorPayload({
  code: "unauthorized",
  message: "A valid Bearer token is required.",
  hint: "POST /api/auth/otp/start, then POST /api/auth/otp/verify, and send Authorization: Bearer <token>.",
});

const errorSchema = {
  type: "object",
  additionalProperties: false,
  required: ["error", "code", "message", "hint"],
  properties: {
    error: {
      type: "string",
      description: "Human-readable error message (same as message, for existing clients).",
    },
    code: {
      type: "string",
      description: "Stable machine-readable error code.",
    },
    message: {
      type: "string",
      description: "Human-readable error message.",
    },
    hint: {
      type: "string",
      description: "What to do next to resolve the error.",
    },
  },
} as const;

const healthCheckSchema = {
  type: "object",
  additionalProperties: false,
  required: ["ok"],
  properties: {
    ok: { type: "boolean" },
    detail: { type: "string" },
  },
} as const;

const healthResponseSchema = {
  type: "object",
  additionalProperties: false,
  required: ["ok", "service", "status", "checks", "timestamp"],
  properties: {
    ok: { type: "boolean" },
    service: { type: "string" },
    status: { type: "string", enum: ["healthy", "degraded", "unhealthy"] },
    checks: {
      type: "object",
      additionalProperties: healthCheckSchema,
    },
    timestamp: { type: "string", format: "date-time" },
  },
} as const;

function jsonErrorResponse(description: string) {
  return {
    description,
    content: {
      "application/json": {
        schema: { $ref: "#/components/schemas/ApiError" },
        example: errorExample,
      },
    },
  };
}

export const openApiSpec = {
  openapi: "3.1.0",
  info: {
    title: "Bast.sh HTTP API",
    version: "1.0.0",
    summary: "Hosted Bast.sh APIs for Vault, health, docs search, telemetry, and CLI error reports.",
    description:
      "Bast.sh is a native SSH picker and key manager. This spec describes the hosted HTTP API on bast.sh. Host and key automation for local OpenSSH config uses the Bast.sh CLI (`bast --json`), not these endpoints. Vault encrypts data on the client before upload.",
    contact: {
      name: company.tradingName,
      email: company.legalEmail,
      url: `${siteUrl}/contact`,
    },
    license: {
      name: "MIT (CLI); hosted API terms apply",
      url: `${siteUrl}/legal/terms`,
    },
  },
  servers: [
    {
      url: siteUrl,
      description: "Production Bast.sh",
    },
  ],
  tags: [
    {
      name: "Health",
      description: "Liveness and dependency checks for the marketing site, docs, and Vault backends.",
    },
    {
      name: "Vault",
      description: "End-to-end encrypted host/key sync. Authenticate with email OTP, then Bearer tokens.",
    },
    {
      name: "Docs",
      description: "Documentation search used by the Bast.sh docs UI.",
    },
    {
      name: "Telemetry",
      description: "Anonymous CLI and installer events. Disable in the CLI with BAST_NO_TELEMETRY=1.",
    },
  ],
  components: {
    securitySchemes: {
      bearerAuth: {
        type: "http",
        scheme: "bearer",
        bearerFormat: "opaque",
        description:
          "Vault session token from POST /api/auth/otp/verify. Send `Authorization: Bearer <token>`.",
      },
    },
    schemas: {
      ApiError: errorSchema,
      HealthCheck: healthCheckSchema,
      HealthResponse: healthResponseSchema,
      OtpStartRequest: {
        type: "object",
        additionalProperties: false,
        required: ["email"],
        properties: {
          email: {
            type: "string",
            format: "email",
            description: "Account email. A 6-digit code is sent to this address.",
          },
          acceptTerms: {
            type: "boolean",
            description:
              "Required true on hosted bast.sh. Confirms the Terms of Service and Privacy Policy.",
          },
        },
      },
      OtpStartResponse: {
        type: "object",
        additionalProperties: false,
        required: ["ok"],
        properties: { ok: { type: "boolean", enum: [true] } },
      },
      OtpVerifyRequest: {
        type: "object",
        additionalProperties: false,
        required: ["email", "code"],
        properties: {
          email: { type: "string", format: "email" },
          code: {
            type: "string",
            pattern: "^[0-9]{6}$",
            description: "6-digit one-time code.",
          },
        },
      },
      OtpVerifyResponse: {
        type: "object",
        additionalProperties: false,
        required: ["ok", "token", "userId", "email", "deviceId"],
        properties: {
          ok: { type: "boolean", enum: [true] },
          token: { type: "string", description: "Bearer token for /api/vault." },
          userId: { type: "string" },
          email: { type: "string", format: "email" },
          deviceId: { type: "string" },
        },
      },
      LogoutResponse: {
        type: "object",
        additionalProperties: false,
        required: ["ok"],
        properties: { ok: { type: "boolean", enum: [true] } },
      },
      VaultMeta: {
        type: "object",
        additionalProperties: false,
        required: ["revision", "updatedAt", "size", "contentHash"],
        properties: {
          revision: { type: "string" },
          updatedAt: { type: "integer" },
          size: { type: "integer" },
          contentHash: { type: "string" },
        },
      },
      TelemetryPayload: {
        type: "object",
        additionalProperties: false,
        required: ["event", "version", "os", "arch", "source"],
        properties: {
          event: { type: "string" },
          version: {
            type: "string",
            description: "CLI version such as v0.9.2, nightly.YYYYMMDD.sha, or dev.",
          },
          os: { type: "string", enum: ["darwin", "linux"] },
          arch: { type: "string", enum: ["arm64", "amd64"] },
          source: { type: "string", enum: ["installer", "cli"] },
        },
      },
      ErrorReportPayload: {
        type: "object",
        additionalProperties: false,
        required: ["message", "version", "os", "arch", "source"],
        properties: {
          message: { type: "string" },
          version: { type: "string" },
          os: { type: "string", enum: ["darwin", "linux"] },
          arch: { type: "string", enum: ["arm64", "amd64"] },
          source: { type: "string", enum: ["cli"] },
          code: { type: "string" },
          stack: { type: "string" },
          context: {
            type: "string",
            enum: ["tui", "cli", "panic", "connect_prepare"],
          },
          command: { type: "string" },
        },
      },
      DocsSearchHit: {
        type: "object",
        additionalProperties: true,
        properties: {
          id: { type: "string" },
          url: { type: "string" },
          content: { type: "string" },
        },
      },
    },
  },
  paths: {
    "/api/health": {
      get: {
        operationId: "getMarketingHealth",
        tags: ["Health"],
        summary: "Marketing site health",
        description:
          "Liveness check for the Bast.sh marketing app. Returns JSON even when degraded.",
        responses: {
          "200": {
            description: "The marketing app is healthy.",
            content: {
              "application/json": {
                schema: { $ref: "#/components/schemas/HealthResponse" },
              },
            },
          },
          "503": {
            description: "The marketing app is unhealthy.",
            content: {
              "application/json": {
                schema: { $ref: "#/components/schemas/HealthResponse" },
              },
            },
          },
        },
      },
    },
    "/api/health/docs": {
      get: {
        operationId: "getDocsHealth",
        tags: ["Health"],
        summary: "Documentation source health",
        description: "Checks that the Bast.sh docs source and page tree loaded.",
        responses: {
          "200": {
            description: "Docs source is healthy.",
            content: {
              "application/json": {
                schema: { $ref: "#/components/schemas/HealthResponse" },
              },
            },
          },
          "503": {
            description: "Docs source failed to load.",
            content: {
              "application/json": {
                schema: { $ref: "#/components/schemas/HealthResponse" },
              },
            },
          },
        },
      },
    },
    "/api/health/vault": {
      get: {
        operationId: "getVaultHealth",
        tags: ["Health"],
        summary: "Vault backend health",
        description:
          "Pings Upstash Redis and probes Cloudflare R2 used by Bast Vault.",
        responses: {
          "200": {
            description: "Vault backends are healthy.",
            content: {
              "application/json": {
                schema: { $ref: "#/components/schemas/HealthResponse" },
              },
            },
          },
          "503": {
            description: "Vault backends are missing, degraded, or unreachable.",
            content: {
              "application/json": {
                schema: { $ref: "#/components/schemas/HealthResponse" },
              },
            },
          },
        },
      },
    },
    "/api/auth/otp/start": {
      post: {
        operationId: "startVaultOtp",
        tags: ["Vault"],
        summary: "Start Vault email OTP",
        description:
          "Send a 6-digit sign-in code to an email address. On hosted bast.sh, acceptTerms must be true. Rate limited per email and IP.",
        requestBody: {
          required: true,
          content: {
            "application/json": {
              schema: { $ref: "#/components/schemas/OtpStartRequest" },
            },
          },
        },
        responses: {
          "200": {
            description: "A code was sent (or queued).",
            content: {
              "application/json": {
                schema: { $ref: "#/components/schemas/OtpStartResponse" },
              },
            },
          },
          "400": jsonErrorResponse("Invalid JSON, email, or missing terms acceptance."),
          "429": jsonErrorResponse("Too many codes requested. Wait and retry."),
          "503": jsonErrorResponse("Vault auth is not configured on this origin."),
        },
      },
    },
    "/api/auth/otp/verify": {
      post: {
        operationId: "verifyVaultOtp",
        tags: ["Vault"],
        summary: "Verify Vault email OTP",
        description:
          "Exchange the 6-digit code for a Bearer token. Tokens last 90 days or until logout.",
        requestBody: {
          required: true,
          content: {
            "application/json": {
              schema: { $ref: "#/components/schemas/OtpVerifyRequest" },
            },
          },
        },
        responses: {
          "200": {
            description: "Verified. Use token as Authorization: Bearer.",
            content: {
              "application/json": {
                schema: { $ref: "#/components/schemas/OtpVerifyResponse" },
              },
            },
          },
          "400": jsonErrorResponse("Invalid JSON, or email and code missing."),
          "401": jsonErrorResponse("Invalid or expired code."),
          "503": jsonErrorResponse("Vault auth is not configured on this origin."),
        },
      },
    },
    "/api/auth/logout": {
      post: {
        operationId: "logoutVaultSession",
        tags: ["Vault"],
        summary: "Revoke a Vault Bearer token",
        description:
          "Revokes the presented Bearer token. Idempotent: missing or unknown tokens still return ok.",
        security: [{ bearerAuth: [] }],
        responses: {
          "200": {
            description: "Token revoked or already invalid.",
            content: {
              "application/json": {
                schema: { $ref: "#/components/schemas/LogoutResponse" },
              },
            },
          },
          "503": jsonErrorResponse("Vault auth is not configured on this origin."),
        },
      },
    },
    "/api/vault": {
      get: {
        operationId: "getVault",
        tags: ["Vault"],
        summary: "Download Vault ciphertext",
        description:
          "Returns the opaque Vault blob for the authenticated user. Use If-None-Match with the current revision to receive 304. An empty vault is 404, not an empty body with 200.",
        security: [{ bearerAuth: [] }],
        parameters: [
          {
            name: "If-None-Match",
            in: "header",
            required: false,
            description: "Vault revision ETag from a previous GET or PUT.",
            schema: { type: "string" },
          },
        ],
        responses: {
          "200": {
            description: "Ciphertext blob. Treat as opaque JSON produced by the CLI.",
            headers: {
              ETag: {
                description: "Vault revision.",
                schema: { type: "string" },
              },
              "X-Vault-Updated-At": {
                description: "Unix timestamp (ms) of the revision.",
                schema: { type: "string" },
              },
            },
            content: {
              "application/json": {
                schema: { type: "object", additionalProperties: true },
              },
            },
          },
          "304": { description: "Revision has not changed." },
          "401": jsonErrorResponse("Missing or invalid Bearer token."),
          "404": jsonErrorResponse("No vault exists yet. PUT to create one."),
          "500": jsonErrorResponse("Read failed."),
          "503": jsonErrorResponse("Vault storage is not configured."),
        },
      },
      put: {
        operationId: "putVault",
        tags: ["Vault"],
        summary: "Upload Vault ciphertext",
        description:
          "Stores an opaque Vault blob. Maximum size 1 MiB. Send If-Match with the current revision to update; omit If-Match only when creating the first revision. 412 means another device wrote first.",
        security: [{ bearerAuth: [] }],
        parameters: [
          {
            name: "If-Match",
            in: "header",
            required: false,
            description: "Current revision when replacing an existing vault.",
            schema: { type: "string" },
          },
        ],
        requestBody: {
          required: true,
          content: {
            "application/json": {
              schema: { type: "object", additionalProperties: true },
            },
          },
        },
        responses: {
          "200": {
            description: "Vault written.",
            content: {
              "application/json": {
                schema: { $ref: "#/components/schemas/VaultMeta" },
              },
            },
          },
          "400": jsonErrorResponse("Payload exceeds 1 MiB."),
          "401": jsonErrorResponse("Missing or invalid Bearer token."),
          "412": jsonErrorResponse("Revision mismatch. GET, merge, and retry."),
          "500": jsonErrorResponse("Write failed."),
          "503": jsonErrorResponse("Vault storage is not configured."),
        },
      },
    },
    "/api/search": {
      get: {
        operationId: "searchDocs",
        tags: ["Docs"],
        summary: "Search Bast.sh documentation",
        description:
          "Full-text search over the published docs. Used by the docs UI; agents can call it to find pages, then fetch Markdown with Accept: text/markdown on the page URL.",
        parameters: [
          {
            name: "q",
            in: "query",
            required: true,
            description: "Search query.",
            schema: { type: "string" },
          },
        ],
        responses: {
          "200": {
            description: "Search hits.",
            content: {
              "application/json": {
                schema: {
                  type: "array",
                  items: { $ref: "#/components/schemas/DocsSearchHit" },
                },
              },
            },
          },
        },
      },
    },
    "/api/telemetry": {
      post: {
        operationId: "postTelemetry",
        tags: ["Telemetry"],
        summary: "Submit an anonymous usage event",
        description:
          "Called by the Bast.sh installer and CLI. Not required for Vault or docs. Disable with BAST_NO_TELEMETRY=1.",
        requestBody: {
          required: true,
          content: {
            "application/json": {
              schema: { $ref: "#/components/schemas/TelemetryPayload" },
            },
          },
        },
        responses: {
          "204": { description: "Event accepted." },
          "400": jsonErrorResponse("Missing or invalid payload."),
        },
      },
    },
    "/api/errors": {
      post: {
        operationId: "postCliError",
        tags: ["Telemetry"],
        summary: "Submit a consented CLI error report",
        description:
          "Called only after the user agrees at the Bast.sh error prompt. Do not send hostnames, keys, or vault passphrases.",
        requestBody: {
          required: true,
          content: {
            "application/json": {
              schema: { $ref: "#/components/schemas/ErrorReportPayload" },
            },
          },
        },
        responses: {
          "204": { description: "Report accepted." },
          "400": jsonErrorResponse("Missing or invalid payload."),
        },
      },
    },
    "/api/avatar/{username}": {
      get: {
        operationId: "getAvatar",
        tags: ["Docs"],
        summary: "Fetch a cached X avatar",
        description:
          "Used by the Bast.sh testimonials UI. Returns the image bytes, not JSON, unless the username is invalid.",
        parameters: [
          {
            name: "username",
            in: "path",
            required: true,
            description: "X/Twitter handle without @.",
            schema: { type: "string", pattern: "^[A-Za-z0-9_]{1,15}$" },
          },
        ],
        responses: {
          "200": {
            description: "Image bytes.",
            content: {
              "image/jpeg": { schema: { type: "string", format: "binary" } },
              "image/png": { schema: { type: "string", format: "binary" } },
              "image/webp": { schema: { type: "string", format: "binary" } },
            },
          },
          "304": { description: "Cached copy is still valid." },
          "400": jsonErrorResponse("Username is not a valid X handle."),
          "404": { description: "No avatar is stored for this handle." },
        },
      },
    },
  },
} as const;

export function openApiOperationIds(): string[] {
  const ids: string[] = [];
  for (const pathItem of Object.values(openApiSpec.paths)) {
    for (const method of Object.values(pathItem)) {
      if (method && typeof method === "object" && "operationId" in method) {
        ids.push(String(method.operationId));
      }
    }
  }
  return ids;
}
