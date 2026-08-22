import { describe, expect, test } from "bun:test";

import {
  isHostedVaultHost,
  termsAcceptanceError,
  vaultHostFromHeader,
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
});

describe("termsAcceptanceError", () => {
  test("requires acceptance on bast.sh", () => {
    expect(termsAcceptanceError("bast.sh", undefined)).toContain("Terms of Service");
    expect(termsAcceptanceError("www.bast.sh", false)).toContain("Terms of Service");
    expect(termsAcceptanceError("bast.sh", true)).toBeNull();
  });

  test("skips self-hosted hosts", () => {
    expect(termsAcceptanceError("vault.internal", undefined)).toBeNull();
    expect(termsAcceptanceError("localhost:3000", false)).toBeNull();
  });
});
