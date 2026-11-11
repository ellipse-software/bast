import { jsonError } from "@/lib/api-error";
import { parseErrorReportPayload } from "@/lib/errors";
import {
  allowIngest,
  clientIP,
  contentLengthTooLarge,
  ERROR_MAX_BYTES,
  ERROR_RATE_LIMIT,
  INGEST_WINDOW_SECONDS,
  readJsonBody,
} from "@/lib/ingest-limit";
import { captureCliError } from "@/lib/sentry";

function getCountryFromRequest(request: Request): string | undefined {
	const country = request.headers.get("x-vercel-ip-country");
	if (!country || country === "XX" || !/^[A-Z]{2}$/i.test(country)) {
		return undefined;
	}

	return country.toUpperCase();
}

function tooLarge() {
	return jsonError(400, {
		code: "payload_too_large",
		message: "Error report exceeds the 24 KiB limit.",
		hint: "Send a JSON body of at most 24576 bytes. See https://bast.sh/openapi.json.",
	});
}

export async function POST(request: Request) {
	if (contentLengthTooLarge(request, ERROR_MAX_BYTES)) {
		return tooLarge();
	}

	const allowed = await allowIngest({
		bucket: "errors",
		ip: clientIP(request.headers),
		limit: ERROR_RATE_LIMIT,
		windowSeconds: INGEST_WINDOW_SECONDS,
	});
	if (!allowed) {
		return jsonError(429, {
			code: "rate_limited",
			message: "Too many error reports.",
			hint: "Wait a minute and retry. At most 10 reports per minute per IP.",
		});
	}

	const parsed = await readJsonBody(request, ERROR_MAX_BYTES);
	if (!parsed.ok) {
		if (parsed.reason === "too_large") {
			return tooLarge();
		}
		return jsonError(400, {
			code: "invalid_json",
			message: "Request body is not valid JSON.",
			hint: "POST application/json matching the ErrorReportPayload schema in https://bast.sh/openapi.json.",
		});
	}

	const payload = parseErrorReportPayload(parsed.body);
	if (!payload) {
		return jsonError(400, {
			code: "invalid_payload",
			message: "The error report payload was missing or invalid.",
			hint: "Send message, version, os (darwin|linux|windows), arch (arm64|amd64), and source (cli). See https://bast.sh/openapi.json.",
		});
	}

	await captureCliError(payload, getCountryFromRequest(request));

	return new Response(null, { status: 204 });
}
