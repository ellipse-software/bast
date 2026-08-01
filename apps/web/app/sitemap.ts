import type { MetadataRoute } from "next";

import { comparisonSlugs } from "@/lib/comparisons";
import { guideNavItems } from "@/lib/marketing";
import { source } from "@/lib/source";
import { llmsFullUrl, llmsTxtUrl, skillUrl, siteUrl } from "@/lib/site";

export default function sitemap(): MetadataRoute.Sitemap {
  const docPages = source.getPages().map((page) => ({
    url: `${siteUrl}${page.url}`,
    changeFrequency: "monthly" as const,
    priority: page.url === "/docs" ? 0.8 : 0.7,
  }));

  const comparisonPages = comparisonSlugs.map((slug) => ({
    url: `${siteUrl}/${slug}`,
    changeFrequency: "monthly" as const,
    priority: 0.85,
  }));

  const guidePages = guideNavItems.map((item) => ({
    url: `${siteUrl}${item.href}`,
    changeFrequency: "monthly" as const,
    priority: 0.8,
  }));

  return [
    {
      url: siteUrl,
      changeFrequency: "monthly",
      priority: 1,
    },
    {
      url: `${siteUrl}/alternatives`,
      changeFrequency: "monthly",
      priority: 0.9,
    },
    {
      url: `${siteUrl}/features`,
      changeFrequency: "monthly",
      priority: 0.88,
    },
    {
      url: `${siteUrl}/changelog`,
      changeFrequency: "weekly",
      priority: 0.75,
    },
    {
      url: `${siteUrl}/status`,
      changeFrequency: "hourly",
      priority: 0.7,
    },
    ...comparisonPages,
    ...guidePages,
    {
      url: llmsTxtUrl,
      changeFrequency: "monthly",
      priority: 0.6,
    },
    {
      url: llmsFullUrl,
      changeFrequency: "monthly",
      priority: 0.5,
    },
    {
      url: skillUrl,
      changeFrequency: "monthly",
      priority: 0.6,
    },
    ...docPages,
  ];
}
