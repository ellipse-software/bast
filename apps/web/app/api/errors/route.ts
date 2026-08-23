import { jsonError } from "@/lib/api-error";
import { parseErrorReportPayload } from "@/lib/errors";
import { captureCliError } from "@/lib/sentry";

function getCountryFromRequest(request: Request): string | undefined {
	const country = request.headers.get("x-vercel-ip-country");
	if (!country || country === "XX" || !/^[A-Z]{2}$/i.test(country)) {
		return undefined;
	}

	return country.toUpperCase();
}

export async function POST(request: Request) {
	let body: unknown;

	try {
		body = await request.json();
	} catch {
		return jsonError(400, {
			code: "invalid_json",
			message: "Request body is not valid JSON.",
			hint: "POST application/json matching the ErrorReportPayload schema in https://bast.sh/openapi.json.",
		});
	}

	const payload = parseErrorReportPayload(body);
	if (!payload) {
		return jsonError(400, {
			code: "invalid_payload",
			message: "The error report payload was missing or invalid.",
			hint: "Send message, version, os (darwin|linux), arch (arm64|amd64), and source (cli). See https://bast.sh/openapi.json.",
		});
	}

	await captureCliError(payload, getCountryFromRequest(request));

	return new Response(null, { status: 204 });
}
