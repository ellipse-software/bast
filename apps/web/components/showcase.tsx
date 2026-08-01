import {
  Cloud,
  FolderOpen,
  Key,
  Lock,
  Server,
  Smartphone,
  type LucideIcon,
} from "lucide-react";
import { TuiDemo } from "@/components/tui-demo";

const features: {
  title: string;
  description: string;
  icon: LucideIcon;
  className?: string;
}[] = [
  {
    title: "Hosts",
    description:
      "Browse, group, and favorite every host OpenSSH already knows. Search fast, edit metadata, and connect without leaving the terminal.",
    icon: Server,
    className: "sm:col-span-2 lg:col-span-2",
  },
  {
    title: "Keys",
    description:
      "Generate, import, export, and install native OpenSSH keys. Passphrases stay local. No proprietary key store.",
    icon: Key,
  },
  {
    title: "Cloud sync",
    description:
      "Pull live VMs from GCP, AWS, and Azure through each provider CLI. Synced hosts stay read-only and owned by the cloud.",
    icon: Cloud,
  },
  {
    title: "Bast vault",
    description:
      "Encrypt Bast-managed hosts, keys, and metadata on your machine, then sync ciphertext between Macs and Linux boxes.",
    icon: Lock,
  },
  {
    title: "Mobile view",
    description:
      "On phone SSH apps and narrow panes, the list and detail stack so everything stays readable. Tap Connect or keep using the keyboard.",
    icon: Smartphone,
  },
  {
    title: "SFTP",
    description:
      "Dual-pane local and remote browser in the TUI. Copy and move with the same OpenSSH config Connect already uses.",
    icon: FolderOpen,
    className: "sm:col-span-2 lg:col-span-3",
  },
];

export function Showcase() {
  return (
    <div className="w-full max-w-4xl bg-border p-px">
      <div className="bg-background">
        <div className="relative h-[560px] overflow-hidden sm:h-[360px] md:h-[380px]">
          <TuiDemo />
        </div>

        <div className="border-t border-border">
          <div className="grid grid-cols-1 gap-px bg-border sm:grid-cols-2 lg:grid-cols-3">
            {features.map((feature) => {
              const Icon = feature.icon;
              return (
                <article
                  key={feature.title}
                  className={`bg-background p-5 sm:p-6 ${feature.className ?? ""}`}
                >
                  <Icon
                    className="mb-4 size-4 text-accent"
                    strokeWidth={1.5}
                    aria-hidden
                  />
                  <h3 className="mb-2 text-sm font-medium tracking-tight">
                    {feature.title}
                  </h3>
                  <p className="max-w-2xl text-sm leading-relaxed text-muted">
                    {feature.description}
                  </p>
                </article>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}
