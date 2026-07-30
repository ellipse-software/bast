import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { RootProvider } from "fumadocs-ui/provider/next";

import {
  defaultDescription,
  ogImage,
  siteName,
} from "@/lib/metadata";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  metadataBase: new URL("https://bast.sh"),
  title: {
    default: "Bast.sh: native SSH picker and key manager",
    template: "%s | Bast.sh",
  },
  description: defaultDescription,
  applicationName: siteName,
  icons: {
    icon: {
      url: "/favicon.svg",
      type: "image/svg+xml",
    },
  },
  keywords: [
    "SSH client",
    "SSH key manager",
    "SSH host manager",
    "terminal",
    "developer tools",
  ],
  alternates: {
    canonical: "/",
  },
  openGraph: {
    type: "website",
    url: "/",
    siteName,
    title: "Bast.sh: native SSH picker and key manager",
    description: defaultDescription,
    images: [ogImage],
  },
  twitter: {
    card: "summary_large_image",
    title: "Bast.sh: native SSH picker and key manager",
    description: defaultDescription,
    images: [ogImage.url],
  },
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      "max-image-preview": "large",
      "max-snippet": -1,
      "max-video-preview": -1,
    },
  },
};

export const viewport: Viewport = {
  themeColor: "#8B5CF6",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
      suppressHydrationWarning
    >
      <body className="flex min-h-full flex-col bg-background text-foreground">
        <RootProvider theme={{ defaultTheme: "dark", enableSystem: false }}>
          {children}
        </RootProvider>
      </body>
    </html>
  );
}
