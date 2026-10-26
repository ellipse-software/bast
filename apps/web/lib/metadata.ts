import type { Metadata } from "next";

export const siteName = "Bast.sh";

export const defaultDescription =
  "Browse SSH hosts, manage keys, and connect from the terminal. The fast way into the servers you use every day.";

export const ogImage = {
  url: "/og-image.png",
  width: 1800,
  height: 950,
  alt: "The fast way into the servers you use everyday.",
} as const;

export function pageTitle(title: string): string {
  return `${title} | ${siteName}`;
}

export function createPageMetadata({
  title,
  description = defaultDescription,
  path,
}: {
  title: string;
  description?: string;
  path: string;
}): Metadata {
  const openGraphTitle = pageTitle(title);

  return {
    title,
    description,
    alternates: {
      canonical: path,
    },
    openGraph: {
      type: "website",
      url: path,
      siteName,
      title: openGraphTitle,
      description,
      images: [ogImage],
    },
    twitter: {
      card: "summary_large_image",
      title: openGraphTitle,
      description,
      images: [ogImage.url],
    },
  };
}
