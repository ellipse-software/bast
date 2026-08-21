"use server";

import Stripe from "stripe";

import { getStripe, stripeConfigured } from "@/lib/stripe";
import {
  parseSponsorAmountUsd,
  SPONSOR_MESSAGE_MAX,
} from "@/lib/sponsors";
import { normalizeXHandle } from "@/lib/x-handle";

export type SponsorCheckoutInput = {
  amountUsd: number;
  handle?: string;
  message?: string;
  anonymous?: boolean;
};

export async function startSponsorCheckout(
  input: SponsorCheckoutInput,
): Promise<{ clientSecret: string } | { error: string }> {
  if (!stripeConfigured()) {
    return { error: "Payments are not configured yet." };
  }

  const cents = parseSponsorAmountUsd(input.amountUsd);
  if (cents === null) {
    return { error: "Enter an amount between $1 and $10,000." };
  }

  const handle = input.handle?.trim()
    ? normalizeXHandle(input.handle)
    : null;
  if (input.handle?.trim() && !handle) {
    return { error: "Enter a valid X username." };
  }

  const message = input.message?.trim() ?? "";
  if (message.length > SPONSOR_MESSAGE_MAX) {
    return { error: `Keep the note under ${SPONSOR_MESSAGE_MAX} characters.` };
  }

  try {
    const session = await getStripe().checkout.sessions.create({
      ui_mode: "embedded_page",
      redirect_on_completion: "never",
      mode: "payment",
      submit_type: "donate",
      branding_settings: {
        background_color: "#0a0a0a",
        button_color: "#8b5cf6",
        border_style: "rectangular",
        font_family: "inter",
        display_name: "Bast",
      },
      line_items: [
        {
          price_data: {
            currency: "usd",
            product_data: {
              name: "Bast sponsorship",
            },
            unit_amount: cents,
          },
          quantity: 1,
        },
      ],
      metadata: {
        kind: "sponsorship",
        anonymous: input.anonymous ? "true" : "false",
        ...(handle ? { handle } : {}),
        ...(message ? { message } : {}),
      },
      payment_intent_data: {
        metadata: {
          kind: "sponsorship",
          anonymous: input.anonymous ? "true" : "false",
          ...(handle ? { handle } : {}),
          ...(message ? { message } : {}),
        },
      },
    });

    if (!session.client_secret) {
      return { error: "Could not start checkout." };
    }

    return { clientSecret: session.client_secret };
  } catch (error) {
    if (error instanceof Stripe.errors.StripeError && error.message) {
      return { error: error.message };
    }
    return { error: "Could not start checkout." };
  }
}
