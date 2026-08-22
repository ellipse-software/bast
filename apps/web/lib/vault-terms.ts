const HOSTED_VAULT_HOSTS = new Set(["bast.sh", "www.bast.sh"]);

export function vaultHostFromHeader(hostHeader: string): string {
  return hostHeader.split(",")[0].trim().toLowerCase().replace(/:\d+$/, "");
}

export function isHostedVaultHost(hostHeader: string): boolean {
  return HOSTED_VAULT_HOSTS.has(vaultHostFromHeader(hostHeader));
}

export function termsAcceptanceError(
  hostHeader: string,
  acceptTerms: unknown,
): string | null {
  if (!isHostedVaultHost(hostHeader)) {
    return null;
  }
  if (acceptTerms === true) {
    return null;
  }
  return "accept the Terms of Service and Privacy Policy to continue";
}
