import { openApiUrl, siteUrl } from "@/lib/site";

export function GET() {
  return Response.json({
    name: "Bast.sh HTTP API",
    description:
      "Hosted Vault, health, docs search, and telemetry. Local SSH host and key automation uses the Bast.sh CLI.",
    openapi: openApiUrl,
    docs: `${siteUrl}/docs/reference/api`,
    developers: `${siteUrl}/developers`,
    llms: `${siteUrl}/llms.txt`,
  });
}
