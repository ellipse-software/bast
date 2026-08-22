import { describe, expect, test } from "bun:test";

import {
  isHostedVaultHost,
  termsAcceptanceError,
  vaultHostFromHeader,
  vaultRequestHost,
} from "@/lib/vault-terms";

describe("hosted vault host", () => {
  test("matches bast.sh and www", () => {
    expect(isHostedVaultHost("bast.sh")).toBe(true);
    expect(isHostedVaultHost("www.bast.sh")).toBe(true);
    expect(isHostedVaultHost("bast.sh:443")).toBe(true);
    expect(isHostedVaultHost("vault.example.com")).toBe(false);
    expect(isHostedVaultHost("localhost:3000")).toBe(false);
  });

  test("uses the first forwarded host", () => {
    expect(vaultHostFromHeader("bast.sh, localhost")).toBe("bast.sh");
  });

  test("strips a trailing DNS root label", () => {
    expect(vaultHostFromHeader("bast.sh.")).toBe("bast.sh");
    expect(isHostedVaultHost("www.bast.sh.")).toBe(true);
  });
});

describe("vaultRequestHost", () => {
  test("uses Host, not x-forwarded-host", () => {
    const headers = {
      get(name: string) {
        if (name === "host") return "bast.sh";
        if (name === "x-forwarded-host") return "localhost";
        return null;
      },
    };
    expect(vaultRequestHost(headers)).toBe("bast.sh");
  });

  test("treats production deployments as hosted when Host is the gateway", () => {
    const headers = {
      get(name: string) {
        if (name === "host") return "bast-web.vercel.gateway.ellipseusercontent.com";
        if (name === "x-forwarded-host") return "localhost";
        return null;
      },
    };
    expect(
      vaultRequestHost(headers, {
        VERCEL_ENV: "production",
        VERCEL_PROJECT_PRODUCTION_URL: "https://www.bast.sh",
      }),
    ).toBe("www.bast.sh");
  });

  test("does not treat preview or self-host as hosted", () => {
    const headers = {
      get(name: string) {
        if (name === "host") return "vault.example.com";
        return null;
      },
    };
    expect(
      vaultRequestHost(headers, {
        VERCEL_ENV: "production",
        VERCEL_PROJECT_PRODUCTION_URL: "vault.example.com",
      }),
    ).toBe("vault.example.com");
    expect(
      vaultRequestHost(headers, {
        VERCEL_ENV: "preview",
        VERCEL_PROJECT_PRODUCTION_URL: "bast.sh",
      }),
    ).toBe("vault.example.com");
  });
});

describe("termsAcceptanceError", () => {
  test("requires acceptance on bast.sh", () => {
    expect(termsAcceptanceError("bast.sh", undefined)).toContain("Terms of Service");
    expect(termsAcceptanceError("www.bast.sh", false)).toContain("Terms of Service");
    expect(termsAcceptanceError("bast.sh.", undefined)).toContain("Terms of Service");
    expect(termsAcceptanceError("bast.sh", true)).toBeNull();
  });

  test("skips self-hosted hosts", () => {
    expect(termsAcceptanceError("vault.internal", undefined)).toBeNull();
    expect(termsAcceptanceError("localhost:3000", false)).toBeNull();
  });
});
