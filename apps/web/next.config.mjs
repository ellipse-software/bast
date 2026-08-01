import { createMDX } from "fumadocs-mdx/next";

/** @type {import('next').NextConfig} */
const config = {
  reactStrictMode: true,
  // Keep files-sdk out of the Turbopack graph so its optional @aws-sdk/*
  // peers (unused with client: "fetch") are not resolved at build time.
  serverExternalPackages: ["files-sdk"],
  images: {
    remotePatterns: [
      {
        protocol: "https",
        hostname: "cdn.bast.sh",
      },
    ],
  },
};

const withMDX = createMDX();

export default withMDX(config);
