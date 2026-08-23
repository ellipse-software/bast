import type Stripe from "stripe";

import type { SponsorInterval } from "@/lib/sponsors";

export type SponsorCheckoutSessionInput = {
	cents: number;
	interval: SponsorInterval;
	handle: string | null;
	message: string;
	anonymous: boolean;
	returnUrl: string;
};

export function sponsorCheckoutMetadata(
	input: SponsorCheckoutSessionInput,
): Stripe.MetadataParam {
	return {
		kind: "sponsorship",
		interval: input.interval,
		anonymous: input.anonymous ? "true" : "false",
		...(input.handle ? { handle: input.handle } : {}),
		...(input.message ? { message: input.message } : {}),
	};
}

export function buildSponsorCheckoutSession(
	input: SponsorCheckoutSessionInput,
): Stripe.Checkout.SessionCreateParams {
	const metadata = sponsorCheckoutMetadata(input);
	const monthly = input.interval === "month";
	const params: Stripe.Checkout.SessionCreateParams = {
		ui_mode: "elements",
		mode: monthly ? "subscription" : "payment",
		return_url: input.returnUrl,
		adaptive_pricing: { enabled: true },
		line_items: [
			{
				price_data: {
					currency: "usd",
					product_data: {
						name: monthly
							? "Bast monthly sponsorship"
							: "Bast sponsorship",
					},
					unit_amount: input.cents,
					...(monthly ? { recurring: { interval: "month" as const } } : {}),
				},
				quantity: 1,
			},
		],
		metadata,
	};
	if (monthly) {
		params.subscription_data = { metadata };
	} else {
		params.payment_intent_data = { metadata };
	}
	return params;
}
