import { jsonError } from "@/lib/api-error";
import { startEmailOTP } from "@/lib/auth";
import { termsAcceptanceError, vaultRequestHost } from "@/lib/vault-terms";
import { redisConfigured } from "@/lib/redis";

export async function POST(request: Request) {
	if (!redisConfigured()) {
		return jsonError(503, {
			code: "vault_auth_unconfigured",
			message: "Vault auth is not configured on this origin.",
			hint: "Self-host with Upstash Redis or use https://bast.sh. See https://bast.sh/docs/reference/self-hosting.",
		});
	}
	let body: { email?: string; acceptTerms?: boolean };
	try {
		body = await request.json();
	} catch {
		return jsonError(400, {
			code: "invalid_json",
			message: "Request body is not valid JSON.",
			hint: "POST {\"email\":\"you@example.com\",\"acceptTerms\":true}. See https://bast.sh/openapi.json.",
		});
	}
	const email = body.email?.trim();
	if (!email) {
		return jsonError(400, {
			code: "email_required",
			message: "email is required",
			hint: "Include a JSON email field. Hosted bast.sh also requires acceptTerms: true.",
		});
	}
	const host = vaultRequestHost(request.headers);
	const termsError = termsAcceptanceError(host, body.acceptTerms);
	if (termsError) {
		return jsonError(400, {
			code: "terms_required",
			message: termsError,
			hint: "Send acceptTerms: true after reading https://bast.sh/legal/terms and https://bast.sh/privacy.",
		});
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
		const code =
			status === 429
				? "rate_limited"
				: status === 400
					? "otp_rejected"
					: "otp_send_failed";
		return jsonError(status || 400, {
			code,
			message,
			hint:
				status === 429
					? "Wait at least 30 seconds between codes. At most 10 codes per hour per email and IP."
					: "Check the email address and retry. Schema: https://bast.sh/openapi.json.",
		});
	}
}
