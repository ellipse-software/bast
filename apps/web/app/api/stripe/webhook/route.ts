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
    return new Response(null, { status: 503 });
  }

  const signature = request.headers.get("stripe-signature");
  if (!signature) {
    return new Response(null, { status: 400 });
  }

  let event;
  try {
    event = getStripe().webhooks.constructEvent(
      await request.text(),
      signature,
      secret,
    );
  } catch {
    return new Response(null, { status: 400 });
  }

  if (HANDLED.has(event.type)) {
    console.info("[stripe-webhook]", { id: event.id, type: event.type });
  }

  return new Response(null, { status: 200 });
}
