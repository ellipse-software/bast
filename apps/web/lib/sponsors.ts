import { parseXProfileUrl } from "@/lib/x-handle";

/**
 * Landing-page sponsors. Paste an X profile URL, the amount in USD,
 * and an optional message. Name and avatar load from the profile.
 * For sponsors who are not on X, set `name`, a site `href`, and an
 * optional `avatar`. Set `anonymous: true` to hide the name and handle.
 */

export type Sponsor = {
	href: string;
	amount: number;
	message?: string;
	anonymous?: boolean;
	name?: string;
	avatar?: string;
};

export type ResolvedSponsor = {
	href: string;
	amount: number;
	message?: string;
	anonymous?: boolean;
	username: string | null;
	name: string;
	avatar?: string;
};

export const sponsors: Sponsor[] = [
	{
		href: "https://rdke.dev",
		name: "Tom Benjamin Radtke",
		avatar: "https://galileo.ted.ac/dQbmFFHNaO.jpg",
		message: "Finally a clean alternative for people with hundreds of hosts.",
		amount: 250,
	},
	{
		href: "https://x.com/jess_daniel10",
		message: "It’s a great tool, thanks for making it!",
		amount: 100,
	},
	{
		href: "https://x.com/trylle",
		message: "trylle ❤️ bast",
		amount: 100,
	},
	{
		href: "https://x.com/leodev",
		message: "glad to be supporting such a good project. 🙏",
		amount: 20,
	},
];

function httpsUrl(url: string | undefined): string | undefined {
	if (!url) return undefined;
	return url.startsWith("http://") ? `https://${url.slice("http://".length)}` : url;
}

export function resolveSponsors(
	items: readonly Sponsor[],
): ResolvedSponsor[] {
	return items
		.flatMap((sponsor) => {
			const username = parseXProfileUrl(sponsor.href);
			const name = sponsor.name?.trim() || username;
			if (!name) return [];
			return [
				{
					href: sponsor.href,
					amount: sponsor.amount,
					message: sponsor.message,
					anonymous: sponsor.anonymous,
					username,
					name,
					avatar: httpsUrl(sponsor.avatar),
				},
			];
		})
		.sort((a, b) => b.amount - a.amount);
}

/**
 * Remainder of a 3-column grid is promoted: 1 leftover sits as a
 * featured card, 2 leftovers sit as a featured pair, then the rest
 * fill complete rows.
 */
export function partitionSponsorLayout<T>(
	items: readonly T[],
): { featured: T[]; rest: T[] } {
	const leftover = items.length % 3;
	if (leftover === 0) {
		return { featured: [], rest: [...items] };
	}
	return {
		featured: items.slice(0, leftover),
		rest: items.slice(leftover),
	};
}

export const SPONSOR_PRESETS_USD = [5, 10, 25, 50, 100] as const;
export const SPONSOR_MIN_USD = 1;
export const SPONSOR_MAX_USD = 10_000;
export const SPONSOR_MESSAGE_MAX = 280;
export const SPONSOR_INTERVALS = ["once", "month"] as const;

export type SponsorInterval = (typeof SPONSOR_INTERVALS)[number];

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

export function parseSponsorInterval(input: unknown): SponsorInterval | null {
	if (input === undefined || input === null || input === "") {
		return "once";
	}
	if (input === "once" || input === "month") {
		return input;
	}
	return null;
}
