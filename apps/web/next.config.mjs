import { createMDX } from "fumadocs-mdx/next";

/** @type {import('next').NextConfig} */
const config = {
	reactStrictMode: true,
	// Keep files-sdk out of the Turbopack graph so its optional @aws-sdk/*
	// peers (unused with client: "fetch") are not resolved at build time.
	allowedDevOrigins: ["192.168.0.232"],
	serverExternalPackages: ["files-sdk"],
	experimental: {
		serverActions: {
			// Origin is bast.sh; the reverse proxy forwards
			// x-forwarded-host as *.vercel.gateway.ellipseusercontent.com.
			allowedOrigins: [
				"bast.sh",
				"www.bast.sh",
				"**.ellipseusercontent.com",
			],
		},
	},
	images: {
		remotePatterns: [
			{
				protocol: "https",
				hostname: "cdn.bast.sh",
			},
		],
	},
	async headers() {
		return [
			{
				source: "/install.ps1",
				headers: [
					{
						key: "Content-Type",
						value: "text/plain; charset=utf-8",
					},
				],
			},
			{
				source: "/install-nightly.ps1",
				headers: [
					{
						key: "Content-Type",
						value: "text/plain; charset=utf-8",
					},
				],
			},
		];
	},
	async redirects() {
		return [
			{
				source: "/docs/reference/install",
				destination: "/docs/install",
				permanent: true,
			},
		];
	},
};

const withMDX = createMDX();

export default withMDX(config);
