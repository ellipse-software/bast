import type { MetadataRoute } from "next";

import { source } from "@/lib/source";
import { llmsFullUrl, llmsTxtUrl, skillUrl, siteUrl } from "@/lib/site";

export default function sitemap(): MetadataRoute.Sitemap {
  const docPages = source.getPages().map((page) => ({
    url: `${siteUrl}${page.url}`,
    lastModified: new Date(),
    changeFrequency: "monthly" as const,
    priority: page.url === "/docs" ? 0.8 : 0.7,
  }));

  return [
    {
      url: siteUrl,
      lastModified: new Date(),
      changeFrequency: "monthly",
      priority: 1,
    },
    {
      url: llmsTxtUrl,
      lastModified: new Date(),
      changeFrequency: "monthly",
      priority: 0.6,
    },
    {
      url: llmsFullUrl,
      lastModified: new Date(),
      changeFrequency: "monthly",
      priority: 0.5,
    },
    {
      url: skillUrl,
      lastModified: new Date(),
      changeFrequency: "monthly",
      priority: 0.6,
    },
    ...docPages,
  ];
}
