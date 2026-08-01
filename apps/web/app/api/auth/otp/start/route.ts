import { startEmailOTP } from "@/lib/auth";
import { redisConfigured } from "@/lib/redis";

export async function POST(request: Request) {
	if (!redisConfigured()) {
		return Response.json(
			{ error: "vault auth is not configured" },
			{ status: 503 },
		);
	}
	let body: { email?: string };
	try {
		body = await request.json();
	} catch {
		return Response.json({ error: "invalid json" }, { status: 400 });
	}
	const email = body.email?.trim();
	if (!email) {
		return Response.json({ error: "email is required" }, { status: 400 });
	}
	const clientIP =
		request.headers.get("x-forwarded-for")?.split(",")[0]?.trim() ||
		request.headers.get("x-real-ip")?.trim() ||
		"";
	try {
		await startEmailOTP(email, clientIP);
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
