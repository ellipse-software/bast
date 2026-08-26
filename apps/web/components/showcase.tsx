import {
  Cloud,
  FolderOpen,
  Key,
  Lock,
  Server,
  Smartphone,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";

import { TuiDemo } from "@/components/tui-demo";

const features: {
  title: string;
  description: string;
  icon: LucideIcon;
  href: string;
  className?: string;
}[] = [
  {
    title: "Hosts",
    description:
      "Browse, group, and favorite every host OpenSSH already knows. Search fast, edit metadata, and connect without leaving the terminal.",
    icon: Server,
    href: "/docs/features/host-picker",
    className: "sm:col-span-2 lg:col-span-2",
  },
  {
    title: "Keys",
    description:
      "Generate, import, export, and install native OpenSSH keys. Passphrases stay local. No proprietary key store.",
    icon: Key,
    href: "/docs/features/keys",
  },
  {
    title: "Cloud sync",
    description:
      "Pull live hosts from GCP, AWS, Azure, box.ascii.dev, Upstash Box, and Vercel Sandbox. Synced hosts stay read-only and owned by the cloud.",
    icon: Cloud,
    href: "/docs/features/gcp",
  },
  {
    title: "Bast vault",
    description:
      "Encrypt Bast-managed hosts, keys, and metadata on your machine, then sync ciphertext between macOS, Linux, and Windows 11 devices.",
    icon: Lock,
    href: "/docs/features/vault",
  },
  {
    title: "Mobile view",
    description:
      "On phone SSH apps and narrow panes, the list and detail stack so everything stays readable. Tap Connect or keep using the keyboard.",
    icon: Smartphone,
    href: "/docs/features/mobile-view",
  },
  {
    title: "SFTP",
    description:
      "Dual-pane local and remote browser in the TUI. Copy and move with the same OpenSSH config Connect already uses.",
    icon: FolderOpen,
    href: "/docs/features/files",
    className: "sm:col-span-2 lg:col-span-3",
  },
];

export function Showcase() {
  return (
    <div className="w-full bg-border p-px">
      <div className="bg-background">
        <div className="relative h-[560px] overflow-hidden sm:h-[360px] md:h-[380px]">
          <TuiDemo />
        </div>

        <div className="border-t border-border">
          <div className="grid grid-cols-1 gap-px bg-border sm:grid-cols-2 lg:grid-cols-3">
            {features.map((feature) => {
              const Icon = feature.icon;
              return (
                <Link
                  key={feature.title}
                  href={feature.href}
                  className={`group relative bg-background p-5 outline-none transition-[background-color,box-shadow] duration-150 sm:p-6 hover:bg-[color-mix(in_srgb,var(--accent)_9%,var(--background))] hover:ring-1 hover:ring-inset hover:ring-accent focus-visible:bg-[color-mix(in_srgb,var(--accent)_9%,var(--background))] focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-accent ${feature.className ?? ""}`}
                >
                  <Icon
                    className="mb-4 size-4 text-accent"
                    strokeWidth={1.5}
                    aria-hidden
                  />
                  <h3 className="mb-2 text-sm font-medium tracking-tight text-foreground">
                    {feature.title}
                  </h3>
                  <p className="max-w-2xl text-sm leading-relaxed text-muted transition-colors group-hover:text-foreground/75">
                    {feature.description}
                  </p>
                </Link>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}
