const HOSTED_VAULT_HOSTS = new Set(["bast.sh", "www.bast.sh"]);

type HeaderReader = { get(name: string): string | null };

export function vaultHostFromHeader(hostHeader: string): string {
  let host = hostHeader.split(",")[0]?.trim().toLowerCase() ?? "";
  host = host.replace(/^https?:\/\//, "");
  host = host.split("/")[0] ?? host;
  host = host.replace(/:\d+$/, "");
  return host.replace(/\.$/, "");
}

export function isHostedVaultHost(hostHeader: string): boolean {
  return HOSTED_VAULT_HOSTS.has(vaultHostFromHeader(hostHeader));
}

export function vaultRequestHost(
  headers: HeaderReader,
  env: Record<string, string | undefined> = process.env,
): string {
  const host = vaultHostFromHeader(headers.get("host") ?? "");
  if (HOSTED_VAULT_HOSTS.has(host)) {
    return host;
  }
  const production = vaultHostFromHeader(env.VERCEL_PROJECT_PRODUCTION_URL ?? "");
  if (env.VERCEL_ENV === "production" && HOSTED_VAULT_HOSTS.has(production)) {
    return production;
  }
  return host;
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
