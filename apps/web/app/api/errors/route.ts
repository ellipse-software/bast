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
		return new Response(null, { status: 400 });
	}

	const payload = parseErrorReportPayload(body);
	if (!payload) {
		return new Response(null, { status: 400 });
	}

	await captureCliError(payload, getCountryFromRequest(request));

	return new Response(null, { status: 204 });
}
