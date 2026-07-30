import {
  Cloud,
  FileText,
  Key,
  Smartphone,
  Tags,
  Terminal,
  type LucideIcon,
} from "lucide-react";
import { TuiDemo } from "@/components/tui-demo";

const features: {
  title: string;
  description: string;
  icon: LucideIcon;
}[] = [
  {
    title: "Native OpenSSH",
    description:
      "Launches the ssh binary already on your machine. No proprietary runtime, no hidden host database.",
    icon: Terminal,
  },
  {
    title: "Your SSH config",
    description:
      "Reads ~/.ssh/config and Include files. Adds one managed Include. Your config stays authoritative.",
    icon: FileText,
  },
  {
    title: "Cloud sync",
    description:
      "Import VMs from GCP, AWS, and Azure. Uses each provider's native CLI. Details stay owned by the cloud.",
    icon: Cloud,
  },
  {
    title: "Mobile UI",
    description:
      "Works in phone SSH apps and narrow terminals. List and detail stack. Tap or use the keyboard.",
    icon: Smartphone,
  },
  {
    title: "Groups, tags & colors",
    description:
      "Nested groups, tags, environment, notes, and hex color labels. Search and sort across all of it.",
    icon: Tags,
  },
  {
    title: "Key management",
    description:
      "Generate, import, and export native SSH keys locally. Verify pairs and change passphrases.",
    icon: Key,
  },
];

export function Showcase() {
  return (
    <div className="w-full max-w-4xl bg-border p-px">
      <div className="bg-background">
        <div className="relative h-[520px] overflow-hidden sm:h-[340px] md:h-[360px]">
          <TuiDemo />
        </div>

        <div className="border-t border-border">
          <div className="grid grid-cols-1 gap-px bg-border sm:grid-cols-2 lg:grid-cols-3">
            {features.map((feature) => {
              const Icon = feature.icon;
              return (
                <article key={feature.title} className="bg-background p-5 sm:p-6">
                  <Icon
                    className="mb-4 size-4 text-accent"
                    strokeWidth={1.5}
                    aria-hidden
                  />
                  <h3 className="mb-2 text-sm font-medium tracking-tight">
                    {feature.title}
                  </h3>
                  <p className="text-sm leading-relaxed text-muted">
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
