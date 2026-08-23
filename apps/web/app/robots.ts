import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: "*",
      allow: "/",
      disallow: ["/markdown"],
    },
    sitemap: "https://bast.sh/sitemap.xml",
    host: "https://bast.sh",
  };
}
