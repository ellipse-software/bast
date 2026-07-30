import type { MetadataRoute } from "next";

import { source } from "@/lib/source";
import { llmsFullUrl, llmsTxtUrl, skillUrl, siteUrl } from "@/lib/site";

export default function sitemap(): MetadataRoute.Sitemap {
  const docPages = source.getPages().map((page) => ({
    url: `${siteUrl}${page.url}`,
    changeFrequency: "monthly" as const,
    priority: page.url === "/docs" ? 0.8 : 0.7,
  }));

  return [
    {
      url: siteUrl,
      changeFrequency: "monthly",
      priority: 1,
    },
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
