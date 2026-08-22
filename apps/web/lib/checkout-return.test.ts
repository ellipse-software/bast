import { describe, expect, test } from "bun:test";

import {
  buildCheckoutReturnUrl,
  checkoutOrigin,
  isCheckoutSessionId,
  parseSponsorDialogIntent,
  requestOriginFromHeaders,
  sanitizeCheckoutReturnPath,
  stripSponsorDialogSearch,
} from "@/lib/checkout-return";
import { siteUrl } from "@/lib/site";

describe("isCheckoutSessionId", () => {
  test("accepts live and test session ids", () => {
    expect(isCheckoutSessionId("cs_test_a1B2c3")).toBe(true);
    expect(isCheckoutSessionId("cs_live_abcDEF123")).toBe(true);
  });

  test("rejects other values", () => {
    expect(isCheckoutSessionId("")).toBe(false);
    expect(isCheckoutSessionId("cs_abc")).toBe(false);
    expect(isCheckoutSessionId("pi_test_123")).toBe(false);
    expect(isCheckoutSessionId("cs_test_abc/../x")).toBe(false);
  });
});

describe("sanitizeCheckoutReturnPath", () => {
  test("keeps site paths", () => {
    expect(sanitizeCheckoutReturnPath("/")).toBe("/");
    expect(sanitizeCheckoutReturnPath("/docs/install")).toBe("/docs/install");
    expect(sanitizeCheckoutReturnPath("/features")).toBe("/features");
  });

  test("strips query and hash", () => {
    expect(sanitizeCheckoutReturnPath("/docs?x=1#y")).toBe("/docs");
  });

  test("rejects open redirects and junk", () => {
    expect(sanitizeCheckoutReturnPath("//evil.test")).toBe("/");
    expect(sanitizeCheckoutReturnPath("/\\evil")).toBe("/");
    expect(sanitizeCheckoutReturnPath("https://evil.test")).toBe("/");
    expect(sanitizeCheckoutReturnPath("/ok%2F..%2F")).toBe("/");
    expect(sanitizeCheckoutReturnPath(null)).toBe("/");
    expect(sanitizeCheckoutReturnPath("")).toBe("/");
  });
});

describe("checkoutOrigin", () => {
  test("uses the public site in production", () => {
    expect(
      checkoutOrigin({
        vercelEnv: "production",
        vercelUrl: "bast-git-main-ellipse.vercel.app",
        requestOrigin: "https://evil.test",
      }),
    ).toBe(siteUrl);
  });

  test("uses the Vercel deployment host on preview", () => {
    expect(
      checkoutOrigin({
        vercelEnv: "preview",
        vercelUrl: "bast-git-feat-ellipse.vercel.app",
        requestOrigin: "https://evil.test",
      }),
    ).toBe("https://bast-git-feat-ellipse.vercel.app");
  });

  test("allows local development hosts", () => {
    expect(
      checkoutOrigin({
        vercelEnv: undefined,
        vercelUrl: undefined,
        requestOrigin: "http://localhost:3000",
      }),
    ).toBe("http://localhost:3000");
    expect(
      checkoutOrigin({
        vercelEnv: undefined,
        vercelUrl: undefined,
        requestOrigin: "http://192.168.0.232:3000",
      }),
    ).toBe("http://192.168.0.232:3000");
  });

  test("ignores untrusted request origins", () => {
    expect(
      checkoutOrigin({
        vercelEnv: undefined,
        vercelUrl: undefined,
        requestOrigin: "https://evil.test",
      }),
    ).toBe(siteUrl);
  });
});

describe("requestOriginFromHeaders", () => {
  test("prefers the first forwarded host", () => {
    expect(
      requestOriginFromHeaders({
        forwardedHost: "localhost:3000, proxy.internal",
        host: "ignored",
        forwardedProto: "http, https",
      }),
    ).toBe("http://localhost:3000");
  });
});

describe("buildCheckoutReturnUrl", () => {
  test("keeps the Stripe session placeholder unencoded", () => {
    expect(buildCheckoutReturnUrl("https://bast.sh", "/docs")).toBe(
      "https://bast.sh/docs?sponsor=complete&session_id={CHECKOUT_SESSION_ID}",
    );
  });
});

describe("parseSponsorDialogIntent", () => {
  test("opens the dialog from /?sponsor=open", () => {
    expect(parseSponsorDialogIntent("?sponsor=open")).toEqual({
      open: true,
      sessionId: null,
    });
  });

  test("resumes a completed checkout session", () => {
    expect(
      parseSponsorDialogIntent(
        "?sponsor=complete&session_id=cs_test_abc123",
      ),
    ).toEqual({
      open: true,
      sessionId: "cs_test_abc123",
    });
  });

  test("ignores unrelated queries", () => {
    expect(parseSponsorDialogIntent("")).toEqual({
      open: false,
      sessionId: null,
    });
    expect(parseSponsorDialogIntent("?foo=1")).toEqual({
      open: false,
      sessionId: null,
    });
  });
});

describe("stripSponsorDialogSearch", () => {
  test("removes sponsor open and complete params", () => {
    expect(stripSponsorDialogSearch("?sponsor=open")).toBe("");
    expect(
      stripSponsorDialogSearch(
        "?sponsor=complete&session_id=cs_test_abc123&keep=1",
      ),
    ).toBe("keep=1");
  });
});
