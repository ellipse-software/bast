"use server";

import { headers } from "next/headers";
import Stripe from "stripe";

import {
  buildCheckoutReturnUrl,
  checkoutOrigin,
  isCheckoutSessionId,
  requestOriginFromHeaders,
  sanitizeCheckoutReturnPath,
} from "@/lib/checkout-return";
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
  returnPath?: string;
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
      ui_mode: "elements",
      mode: "payment",
      return_url: await sponsorCheckoutReturnUrl(input.returnPath),
      adaptive_pricing: { enabled: true },
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

export async function getSponsorCheckoutStatus(
  sessionId: string,
): Promise<{ status: "complete" | "open" | "expired" } | { error: string }> {
  if (!stripeConfigured()) {
    return { error: "Payments are not configured yet." };
  }
  if (!isCheckoutSessionId(sessionId)) {
    return { error: "Invalid checkout session." };
  }

  try {
    const session = await getStripe().checkout.sessions.retrieve(sessionId);
    if (session.metadata?.kind !== "sponsorship") {
      return { error: "Invalid checkout session." };
    }
    if (
      session.status === "complete" ||
      session.status === "open" ||
      session.status === "expired"
    ) {
      return { status: session.status };
    }
    return { error: "Could not verify payment." };
  } catch {
    return { error: "Could not verify payment." };
  }
}

async function sponsorCheckoutReturnUrl(returnPath: string | undefined): Promise<string> {
  const h = await headers();
  const origin = checkoutOrigin({
    vercelEnv: process.env.VERCEL_ENV,
    vercelUrl: process.env.VERCEL_URL,
    requestOrigin: requestOriginFromHeaders({
      forwardedHost: h.get("x-forwarded-host"),
      host: h.get("host"),
      forwardedProto: h.get("x-forwarded-proto"),
    }),
  });
  return buildCheckoutReturnUrl(origin, sanitizeCheckoutReturnPath(returnPath));
}
