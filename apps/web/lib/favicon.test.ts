import { describe, expect, test } from "bun:test";

import {
  FAVICON_PREVIEW_ACCENT,
  FAVICON_PRODUCTION_ACCENT,
  faviconAccent,
  isProductionDeployment,
  renderFaviconSvg,
} from "@/lib/favicon";

describe("faviconAccent", () => {
  test("keeps purple on Vercel production", () => {
    expect(isProductionDeployment("production")).toBe(true);
    expect(faviconAccent("production")).toBe(FAVICON_PRODUCTION_ACCENT);
  });

  test("uses the preview accent off production", () => {
    expect(faviconAccent("preview")).toBe(FAVICON_PREVIEW_ACCENT);
    expect(faviconAccent("development")).toBe(FAVICON_PREVIEW_ACCENT);
    expect(faviconAccent(undefined)).toBe(FAVICON_PREVIEW_ACCENT);
  });
});

describe("renderFaviconSvg", () => {
  test("recolors the production accent", () => {
    const source = `<path fill="${FAVICON_PRODUCTION_ACCENT}"/>`;
    expect(renderFaviconSvg(source, FAVICON_PREVIEW_ACCENT)).toBe(
      `<path fill="${FAVICON_PREVIEW_ACCENT}"/>`,
    );
    expect(renderFaviconSvg(source, FAVICON_PRODUCTION_ACCENT)).toBe(source);
  });
});
