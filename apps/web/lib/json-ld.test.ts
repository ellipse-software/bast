import { describe, expect, test } from "bun:test";

import {
  identityGraphJsonLd,
  organizationJsonLd,
  softwareApplicationJsonLd,
} from "@/lib/json-ld";

describe("identity JSON-LD", () => {
  test("includes Organization with contactPoint and PostalAddress", () => {
    const org = organizationJsonLd();
    expect(org["@type"]).toBe("Organization");
    expect(org.email).toContain("@");
    expect(org.address["@type"]).toBe("PostalAddress");
    expect(org.address.streetAddress.length).toBeGreaterThan(10);
    expect(org.address.addressCountry).toBe("GB");
    expect(org.contactPoint.length).toBeGreaterThanOrEqual(2);
    expect(org.contactPoint[0]?.email).toContain("@");
    expect(org.contactPoint[0]?.contactType.length).toBeGreaterThan(0);
  });

  test("includes SoftwareApplication identity", () => {
    const software = softwareApplicationJsonLd();
    expect(software["@type"]).toBe("SoftwareApplication");
    expect(software.name).toBe("Bast.sh");
    expect(software.url).toBe("https://bast.sh");
    expect(software.offers.price).toBe("0");
  });

  test("graph lists Organization, SoftwareApplication, and WebSite", () => {
    const graph = identityGraphJsonLd()["@graph"].map(
      (node) => node["@type"],
    );
    expect(graph).toEqual(["Organization", "SoftwareApplication", "WebSite"]);
  });
});
