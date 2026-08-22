/**
 * Landing-page sponsors. Paste an X profile URL, the amount in USD,
 * and an optional message. Name and avatar load from the profile.
 * Set `anonymous: true` to hide the name and handle.
 */
export type Sponsor = {
	href: string;
	amount: number;
	message?: string;
	anonymous?: boolean;
};

export const sponsors: Sponsor[] = [
	{
		href: "https://x.com/jess_daniel10",
		message: "It’s a great tool, thanks for making it!",
		amount: 100,
	},
	{
		href: "https://x.com/trylle",
		message: "Trylle ❤️ Bast",
		amount: 100,
	},
];

export const SPONSOR_PRESETS_USD = [5, 10, 25, 50, 100] as const;
export const SPONSOR_MIN_USD = 1;
export const SPONSOR_MAX_USD = 10_000;
export const SPONSOR_MESSAGE_MAX = 280;

export function formatUsd(amount: number): string {
	return new Intl.NumberFormat("en-US", {
		style: "currency",
		currency: "USD",
		maximumFractionDigits: amount % 1 === 0 ? 0 : 2,
	}).format(amount);
}

export function usdToCents(usd: number): number {
	return Math.round(usd * 100);
}

export function parseSponsorAmountUsd(input: unknown): number | null {
	if (typeof input === "string" && input.trim() !== "") {
		const parsed = Number(input);
		if (!Number.isFinite(parsed)) return null;
		input = parsed;
	}
	if (typeof input !== "number" || !Number.isFinite(input)) return null;
	const cents = usdToCents(input);
	if (
		cents < usdToCents(SPONSOR_MIN_USD) ||
		cents > usdToCents(SPONSOR_MAX_USD)
	) {
		return null;
	}
	return cents;
}
