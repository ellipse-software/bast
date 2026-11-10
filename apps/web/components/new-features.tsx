import Link from "next/link";

import { showcaseFeatures } from "@/components/showcase";

export function FeatureSection() {
  return (
    <section className="bg-[#05080f] text-[#e8eef6]">
      <div className="mx-auto w-full max-w-[90rem] px-4 pb-4 sm:px-6 xl:px-10">
        <h2 className="sr-only">Features</h2>
        <div className="overflow-hidden rounded-xl border border-white/15">
          <div className="grid grid-cols-1 gap-px bg-white/15 sm:grid-cols-2 lg:grid-cols-3">
            {showcaseFeatures.map((feature) => {
              const Icon = feature.icon;
              return (
                <Link
                  key={feature.href}
                  href={feature.href}
                  className={`group bg-[#05080f] p-6 outline-none transition-colors hover:bg-[#0c0f14] focus-visible:bg-[#0c0f14] focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-[#c6b6f2] sm:p-7 ${feature.className ?? ""}`}
                >
                  <Icon
                    className="mb-5 size-4 text-[#c6b6f2]"
                    strokeWidth={1.5}
                    aria-hidden
                  />
                  <h3 className="text-base font-medium tracking-tight text-[#e8eef6]">
                    {feature.title}
                  </h3>
                  <p className="mt-2 max-w-2xl text-sm leading-relaxed text-[#9aa8bc] transition-colors group-hover:text-[#c9d5e6]">
                    {feature.description}
                  </p>
                </Link>
              );
            })}
          </div>
        </div>
      </div>
    </section>
  );
}
