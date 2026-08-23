import { describe, expect, test } from "bun:test";

import { buildSponsorCheckoutSession } from "@/lib/sponsor-checkout";

const base = {
  cents: 2500,
  handle: "tedbrine",
  message: "thanks",
  anonymous: false,
  returnUrl: "https://bast.sh/?sponsor=complete",
};

describe("buildSponsorCheckoutSession", () => {
  test("one-time checkout uses payment mode", () => {
    const session = buildSponsorCheckoutSession({ ...base, interval: "once" });
    expect(session.mode).toBe("payment");
    expect(session.line_items?.[0]).toMatchObject({
      price_data: { unit_amount: 2500, currency: "usd" },
    });
    expect(
      session.line_items?.[0] &&
        "price_data" in session.line_items[0] &&
        session.line_items[0].price_data &&
        "recurring" in session.line_items[0].price_data
        ? session.line_items[0].price_data.recurring
        : undefined,
    ).toBeUndefined();
    expect(session.payment_intent_data?.metadata).toMatchObject({
      kind: "sponsorship",
      interval: "once",
    });
    expect(session.subscription_data).toBeUndefined();
  });

  test("monthly checkout uses subscription mode", () => {
    const session = buildSponsorCheckoutSession({ ...base, interval: "month" });
    expect(session.mode).toBe("subscription");
    expect(session.line_items?.[0]).toMatchObject({
      price_data: {
        unit_amount: 2500,
        recurring: { interval: "month" },
      },
    });
    expect(session.subscription_data?.metadata).toMatchObject({
      kind: "sponsorship",
      interval: "month",
    });
    expect(session.payment_intent_data).toBeUndefined();
  });
});
