import { healthResponse, type HealthCheckResult } from "@/lib/health";
import { source } from "@/lib/source";

export const dynamic = "force-dynamic";

export function GET() {
  const checks: Record<string, HealthCheckResult> = {};

  try {
    const pages = source.getPages();
    const count = pages.length;
    checks.source = {
      ok: count > 0,
      detail:
        count > 0
          ? `${count} docs pages loaded`
          : "docs source loaded with zero pages",
    };

    const tree = source.getPageTree();
    checks.tree = {
      ok: Boolean(tree),
      detail: tree ? "docs page tree available" : "docs page tree missing",
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : "unknown error";
    checks.source = { ok: false, detail: message };
    checks.tree = { ok: false, detail: "unavailable after source failure" };
  }

  return healthResponse("docs", checks);
}
