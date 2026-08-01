import { healthResponse } from "@/lib/health";

export const dynamic = "force-dynamic";

export function GET() {
  return healthResponse("marketing", {
    app: { ok: true, detail: "bast.sh marketing app responding" },
  });
}
