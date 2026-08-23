import { jsonError } from "@/lib/api-error";
import { getStripe, stripeConfigured } from "@/lib/stripe";

const HANDLED = new Set([
  "checkout.session.completed",
  "checkout.session.async_payment_succeeded",
  "checkout.session.async_payment_failed",
  "invoice.paid",
  "invoice.payment_failed",
  "customer.subscription.updated",
  "customer.subscription.deleted",
]);

export async function POST(request: Request) {
  const secret = process.env.STRIPE_WEBHOOK_SECRET;
  if (!stripeConfigured() || !secret) {
    return jsonError(503, {
      code: "webhook_unconfigured",
      message: "Stripe webhooks are not configured on this origin.",
      hint: "This endpoint is for Stripe. Agents should use https://bast.sh/openapi.json instead.",
    });
  }

  const signature = request.headers.get("stripe-signature");
  if (!signature) {
    return jsonError(400, {
      code: "missing_stripe_signature",
      message: "The Stripe-Signature header is required.",
      hint: "This endpoint is a Stripe webhook. Do not call it from an agent.",
    });
  }

  let event;
  try {
    event = getStripe().webhooks.constructEvent(
      await request.text(),
      signature,
      secret,
    );
  } catch {
    return jsonError(400, {
      code: "invalid_stripe_signature",
      message: "The Stripe signature could not be verified.",
      hint: "This endpoint is a Stripe webhook. Do not call it from an agent.",
    });
  }

  if (HANDLED.has(event.type)) {
    console.info("[stripe-webhook]", { id: event.id, type: event.type });
  }

  return new Response(null, { status: 200 });
}
