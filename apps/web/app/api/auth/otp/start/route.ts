import { startEmailOTP } from "@/lib/auth";
import { termsAcceptanceError } from "@/lib/vault-terms";
import { redisConfigured } from "@/lib/redis";

export async function POST(request: Request) {
	if (!redisConfigured()) {
		return Response.json(
			{ error: "vault auth is not configured" },
			{ status: 503 },
		);
	}
	let body: { email?: string; acceptTerms?: boolean };
	try {
		body = await request.json();
	} catch {
		return Response.json({ error: "invalid json" }, { status: 400 });
	}
	const email = body.email?.trim();
	if (!email) {
		return Response.json({ error: "email is required" }, { status: 400 });
	}
	const host =
		request.headers.get("x-forwarded-host") ||
		request.headers.get("host") ||
		"";
	const termsError = termsAcceptanceError(host, body.acceptTerms);
	if (termsError) {
		return Response.json({ error: termsError }, { status: 400 });
	}
	const clientIP =
		request.headers.get("x-forwarded-for")?.split(",")[0]?.trim() ||
		request.headers.get("x-real-ip")?.trim() ||
		"";
	try {
		await startEmailOTP(email, clientIP, { acceptTerms: body.acceptTerms === true });
		return Response.json({ ok: true });
	} catch (error) {
		const status =
			error && typeof error === "object" && "status" in error
				? Number(error.status)
				: 400;
		const message =
			error instanceof Error ? error.message : "failed to send code";
		return Response.json({ error: message }, { status: status || 400 });
	}
}
