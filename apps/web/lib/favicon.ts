export const FAVICON_PRODUCTION_ACCENT = "#8B5CF6";
export const FAVICON_PREVIEW_ACCENT = "#F6885C";

export function isProductionDeployment(
  vercelEnv: string | undefined = process.env.VERCEL_ENV,
): boolean {
  return vercelEnv === "production";
}

export function faviconAccent(
  vercelEnv: string | undefined = process.env.VERCEL_ENV,
): string {
  return isProductionDeployment(vercelEnv)
    ? FAVICON_PRODUCTION_ACCENT
    : FAVICON_PREVIEW_ACCENT;
}

export function renderFaviconSvg(source: string, accent: string): string {
  if (accent === FAVICON_PRODUCTION_ACCENT) return source;
  return source.replaceAll(FAVICON_PRODUCTION_ACCENT, accent);
}
