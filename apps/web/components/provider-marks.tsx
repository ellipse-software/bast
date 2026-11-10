"use client";

// Provider marks, recolored with currentColor.
// Simple Icons CC0: Google Cloud, Upstash, Vercel, Hetzner, Railway, Fly.io,
// and DigitalOcean from the current set. AWS and Azure from simple-icons 11.15.0
// (later releases dropped them). Boat is the mark from boat.dev/icon.svg.
// boxd is boxd.sh/favicon.svg. BitQate is traced from bitqate.com/logo.png.

import { useReducedMotion, useAnimationControls, motion } from "motion/react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";

type Mark = {
  d: string;
  viewBox: string;
  transform?: string;
  rule: "nonzero" | "evenodd";
};

const marks = {
  gcp: {
    d: 'M12.19 2.38a9.344 9.344 0 0 0-9.234 6.893c.053-.02-.055.013 0 0-3.875 2.551-3.922 8.11-.247 10.941l.006-.007-.007.03a6.717 6.717 0 0 0 4.077 1.356h5.173l.03.03h5.192c6.687.053 9.376-8.605 3.835-12.35a9.365 9.365 0 0 0-2.821-4.552l-.043.043.006-.05A9.344 9.344 0 0 0 12.19 2.38zm-.358 4.146c1.244-.04 2.518.368 3.486 1.15a5.186 5.186 0 0 1 1.862 4.078v.518c3.53-.07 3.53 5.262 0 5.193h-5.193l-.008.009v-.04H6.785a2.59 2.59 0 0 1-1.067-.23h.001a2.597 2.597 0 1 1 3.437-3.437l3.013-3.012A6.747 6.747 0 0 0 8.11 8.24c.018-.01.04-.026.054-.023a5.186 5.186 0 0 1 3.67-1.69z',
    viewBox: '0 0 24 24',
    rule: 'nonzero',
  },
  aws: {
    d: 'M6.763 10.036c0 .296.032.535.088.71.064.176.144.368.256.576.04.063.056.127.056.183 0 .08-.048.16-.152.24l-.503.335a.383.383 0 0 1-.208.072c-.08 0-.16-.04-.239-.112a2.47 2.47 0 0 1-.287-.375 6.18 6.18 0 0 1-.248-.471c-.622.734-1.405 1.101-2.347 1.101-.67 0-1.205-.191-1.596-.574-.391-.384-.59-.894-.59-1.533 0-.678.239-1.23.726-1.644.487-.415 1.133-.623 1.955-.623.272 0 .551.024.846.064.296.04.6.104.918.176v-.583c0-.607-.127-1.03-.375-1.277-.255-.248-.686-.367-1.3-.367-.28 0-.568.031-.863.103-.295.072-.583.16-.862.272a2.287 2.287 0 0 1-.28.104.488.488 0 0 1-.127.023c-.112 0-.168-.08-.168-.247v-.391c0-.128.016-.224.056-.28a.597.597 0 0 1 .224-.167c.279-.144.614-.264 1.005-.36a4.84 4.84 0 0 1 1.246-.151c.95 0 1.644.216 2.091.647.439.43.662 1.085.662 1.963v2.586zm-3.24 1.214c.263 0 .534-.048.822-.144.287-.096.543-.271.758-.51.128-.152.224-.32.272-.512.047-.191.08-.423.08-.694v-.335a6.66 6.66 0 0 0-.735-.136 6.02 6.02 0 0 0-.75-.048c-.535 0-.926.104-1.19.32-.263.215-.39.518-.39.917 0 .375.095.655.295.846.191.2.47.296.838.296zm6.41.862c-.144 0-.24-.024-.304-.08-.064-.048-.12-.16-.168-.311L7.586 5.55a1.398 1.398 0 0 1-.072-.32c0-.128.064-.2.191-.2h.783c.151 0 .255.025.31.08.065.048.113.16.16.312l1.342 5.284 1.245-5.284c.04-.16.088-.264.151-.312a.549.549 0 0 1 .32-.08h.638c.152 0 .256.025.32.08.063.048.12.16.151.312l1.261 5.348 1.381-5.348c.048-.16.104-.264.16-.312a.52.52 0 0 1 .311-.08h.743c.127 0 .2.065.2.2 0 .04-.009.08-.017.128a1.137 1.137 0 0 1-.056.2l-1.923 6.17c-.048.16-.104.263-.168.311a.51.51 0 0 1-.303.08h-.687c-.151 0-.255-.024-.32-.08-.063-.056-.119-.16-.15-.32l-1.238-5.148-1.23 5.14c-.04.16-.087.264-.15.32-.065.056-.177.08-.32.08zm10.256.215c-.415 0-.83-.048-1.229-.143-.399-.096-.71-.2-.918-.32-.128-.071-.215-.151-.247-.223a.563.563 0 0 1-.048-.224v-.407c0-.167.064-.247.183-.247.048 0 .096.008.144.024.048.016.12.048.2.08.271.12.566.215.878.279.319.064.63.096.95.096.502 0 .894-.088 1.165-.264a.86.86 0 0 0 .415-.758.777.777 0 0 0-.215-.559c-.144-.151-.416-.287-.807-.415l-1.157-.36c-.583-.183-1.014-.454-1.277-.813a1.902 1.902 0 0 1-.4-1.158c0-.335.073-.63.216-.886.144-.255.335-.479.575-.654.24-.184.51-.32.83-.415.32-.096.655-.136 1.006-.136.175 0 .359.008.535.032.183.024.35.056.518.088.16.04.312.08.455.127.144.048.256.096.336.144a.69.69 0 0 1 .24.2.43.43 0 0 1 .071.263v.375c0 .168-.064.256-.184.256a.83.83 0 0 1-.303-.096 3.652 3.652 0 0 0-1.532-.311c-.455 0-.815.071-1.062.223-.248.152-.375.383-.375.71 0 .224.08.416.24.567.159.152.454.304.877.44l1.134.358c.574.184.99.44 1.237.767.247.327.367.702.367 1.117 0 .343-.072.655-.207.926-.144.272-.336.511-.583.703-.248.2-.543.343-.886.447-.36.111-.734.167-1.142.167zM21.698 16.207c-2.626 1.94-6.442 2.969-9.722 2.969-4.598 0-8.74-1.7-11.87-4.526-.247-.223-.024-.527.272-.351 3.384 1.963 7.559 3.153 11.877 3.153 2.914 0 6.114-.607 9.06-1.852.439-.2.814.287.383.607zM22.792 14.961c-.336-.43-2.22-.207-3.074-.103-.255.032-.295-.192-.063-.36 1.5-1.053 3.967-.75 4.254-.399.287.36-.08 2.826-1.485 4.007-.215.184-.423.088-.327-.151.32-.79 1.03-2.57.695-2.994z',
    viewBox: '0 0 24 24',
    rule: 'nonzero',
  },
  azure: {
    d: 'M22.379 23.343a1.62 1.62 0 0 0 1.536-2.14v.002L17.35 1.76A1.62 1.62 0 0 0 15.816.657H8.184A1.62 1.62 0 0 0 6.65 1.76L.086 21.204a1.62 1.62 0 0 0 1.536 2.139h4.741a1.62 1.62 0 0 0 1.535-1.103l.977-2.892 4.947 3.675c.28.208.618.32.966.32m-3.084-12.531 3.624 10.739a.54.54 0 0 1-.51.713v-.001h-.03a.54.54 0 0 1-.322-.106l-9.287-6.9h4.853m6.313 7.006c.116-.326.13-.694.007-1.058L9.79 1.76a1.722 1.722 0 0 0-.007-.02h6.034a.54.54 0 0 1 .512.366l6.562 19.445a.54.54 0 0 1-.338.684',
    viewBox: '0 0 24 24',
    rule: 'nonzero',
  },
  upstash: {
    d: 'M13.8027 0C11.193 0 8.583.9952 6.5918 2.9863c-3.9823 3.9823-3.9823 10.4396 0 14.4219 1.9911 1.9911 5.2198 1.9911 7.211 0 1.991-1.9911 1.991-5.2198 0-7.211L12 12c.9956.9956.9956 2.6098 0 3.6055-.9956.9955-2.6099.9955-3.6055 0-2.9866-2.9868-2.9866-7.8297 0-10.8164 2.9868-2.9868 7.8297-2.9868 10.8164 0l1.8028-1.8028C19.0225.9952 16.4125 0 13.8027 0zM12 12c-.9956-.9956-.9956-2.6098 0-3.6055.9956-.9955 2.6098-.9955 3.6055 0 2.9867 2.9868 2.9867 7.8297 0 10.8164-2.9867 2.9868-7.8297 2.9868-10.8164 0l-1.8028 1.8028c3.9823 3.9822 10.4396 3.9822 14.4219 0 3.9823-3.9824 3.9823-10.4396 0-14.4219-.9956-.9956-2.3006-1.4922-3.6055-1.4922-1.3048 0-2.6099.4966-3.6054 1.4922-1.9912 1.9912-1.9912 5.2198 0 7.211z',
    viewBox: '0 0 24 24',
    rule: 'nonzero',
  },
  vercel: {
    d: 'm12 1.608 12 20.784H0Z',
    viewBox: '0 0 24 24',
    rule: 'nonzero',
  },
  hetzner: {
    d: 'M0 0v24h24V0H0zm4.602 4.025h2.244c.509 0 .716.215.716.717v5.64h8.883v-5.64c0-.509.215-.717.717-.717h2.229c.5 0 .71.23.724.717v14.516c0 .509-.215.717-.717.717h-2.23c-.51 0-.717-.215-.717-.717v-5.735H7.562v5.735c0 .516-.215.717-.716.717H4.602c-.51 0-.717-.208-.717-.717V4.742c0-.509.207-.717.717-.717z',
    viewBox: '0 0 24 24',
    rule: 'nonzero',
  },
  railway: {
    d: 'M.113 10.27A13.026 13.026 0 000 11.48h18.23c-.064-.125-.15-.237-.235-.347-3.117-4.027-4.793-3.677-7.19-3.78-.8-.034-1.34-.048-4.524-.048-1.704 0-3.555.005-5.358.01-.234.63-.459 1.24-.567 1.737h9.342v1.216H.113v.002zm18.26 2.426H.009c.02.326.05.645.094.961h16.955c.754 0 1.179-.429 1.315-.96zm-17.318 4.28s2.81 6.902 10.93 7.024c4.855 0 9.027-2.883 10.92-7.024H1.056zM11.988 0C7.5 0 3.593 2.466 1.531 6.108l4.75-.005v-.002c3.71 0 3.849.016 4.573.047l.448.016c1.563.052 3.485.22 4.996 1.364.82.621 2.007 1.99 2.712 2.965.654.902.842 1.94.396 2.934-.408.914-1.289 1.458-2.353 1.458H.391s.099.42.249.886h22.748A12.026 12.026 0 0024 12.005C24 5.377 18.621 0 11.988 0z',
    viewBox: '0 0 24 24',
    rule: 'nonzero',
  },
  fly: {
    d: 'M11.987 0c-2.45-.01-5.002.925-6.541 2.897-1.17 1.502-1.664 3.474-1.49 5.356.29 2.112 1.476 3.96 2.676 5.672a41.5 41.5 0 0 0 4.216 4.831c-1.063.832-1.943 2.286-1.357 3.644.821 2.32 4.665 2.05 5.122-.372.39-1.288-.694-2.533-1.428-3.309 2.388-2.431 4.706-5.036 6.17-8.145.595-1.32.902-2.802.614-4.24-.28-2.341-1.823-4.473-3.967-5.46C14.76.266 13.364.016 11.987 0m-.236 1.577v15.534C9.881 13.483 7.724 9.266 8.73 5.069c.35-1.539 1.253-3.309 3.02-3.492m1.996.04c1.534.357 3.031 1.096 3.906 2.48 1.3 1.93 1.318 4.55.1 6.521-1.268 2.395-3.06 4.463-4.916 6.415 1.472-2.974 3.074-6.106 3.182-9.5-.043-2.08-.438-4.612-2.272-5.916M11.97 20.103c.848.342 1.597 1.983.153 2.173-.664.15-1.367-.599-.995-1.222.213-.355.488-.73.842-.95',
    viewBox: '0 0 24 24',
    rule: 'nonzero',
  },
  digitalocean: {
    d: 'M12.04 0C5.408-.02.005 5.37.005 11.992h4.638c0-4.923 4.882-8.731 10.064-6.855a6.95 6.95 0 014.147 4.148c1.889 5.177-1.924 10.055-6.84 10.064v-4.61H7.391v4.623h4.61V24c7.86 0 13.967-7.588 11.397-15.83-1.115-3.59-3.985-6.446-7.575-7.575A12.8 12.8 0 0012.039 0zM7.39 19.362H3.828v3.564H7.39zm-3.563 0v-2.978H.85v2.978z',
    viewBox: '0 0 24 24',
    rule: 'nonzero',
  },
  boat: {
    d: 'M490.123 361.461C494.464 361.461 497.985 364.978 497.985 369.323V403.916C497.985 408.256 494.464 411.777 490.123 411.777H473.317C468.941 411.777 465.396 415.327 465.396 419.699V454.232C465.396 458.573 461.875 462.094 457.534 462.094H84.3579C83.1446 462.094 81.9773 461.511 81.2263 460.558C52.4607 424.054 51.9691 423.595 7.86426 411.777C3.67043 410.653 0 408.256 0 403.916V369.323C0 364.978 3.52235 361.461 7.86426 361.461H490.123ZM234.167 4.81756C238.589 -2.56834 249.679 -1.18229 252.148 7.06463L321.938 240.22L244.366 338.334H51.9617C44.2696 338.334 39.5154 329.947 43.4665 323.347L234.167 4.81756ZM453.056 132.260C451.192 124.166 440.791 121.826 435.640 128.341L282.295 322.292C277.163 328.784 281.786 338.334 290.063 338.334H488.080C494.445 338.334 499.157 332.414 497.728 326.210L453.056 132.260Z',
    viewBox: '0 0 497.985 462.094',
    rule: 'nonzero',
  },
  bitqate: {
    d: 'M7.25 1.00 L9.62 1.00 L9.69 6.77 L10.71 6.23 L11.39 6.02 L13.56 5.96 L15.19 6.30 L16.75 6.98 L16.69 23.00 L14.38 23.00 L14.24 17.23 L13.56 17.64 L12.81 17.91 L11.25 18.11 L10.30 18.04 L9.01 17.77 L7.25 17.09 Z M11.32 8.27 L10.51 9.15 L10.03 10.37 L9.83 11.86 L9.90 13.63 L10.23 14.78 L10.57 15.40 L10.98 15.80 L11.52 16.07 L12.48 15.87 L13.15 15.33 L13.77 14.24 L14.10 12.68 L14.10 10.51 L13.77 9.28 L13.49 8.74 L13.09 8.27 L12.68 7.99 L12.14 7.93 Z',
    viewBox: '0 0 24 24',
    rule: 'evenodd',
  },
  boxd: {
    d: 'M 2 0 L 26 0 A 2 2 0 0 1 28 2 L 28 26 A 2 2 0 0 1 26 28 L 7.6 28 A 2 2 0 0 1 5.6 26 L 5.6 24.4 A 2 2 0 0 0 3.6 22.4 L 2 22.4 A 2 2 0 0 1 0 20.4 L 0 2 A 2 2 0 0 1 2 0 Z',
    viewBox: '0 0 32 32',
    transform: 'translate(2 2)',
    rule: 'nonzero',
  },
} satisfies Record<string, Mark>;

const providers = [
  { id: "gcp", name: "Google Cloud", href: "/docs/features/gcp" },
  { id: "aws", name: "Amazon EC2", href: "/docs/features/aws" },
  { id: "azure", name: "Microsoft Azure", href: "/docs/features/azure" },
  { id: "boat", name: "Boat", href: "/docs/features/boat" },
  { id: "upstash", name: "Upstash", href: "/docs/features/upstash" },
  { id: "vercel", name: "Vercel", href: "/docs/features/vercel" },
  { id: "hetzner", name: "Hetzner Cloud", href: "/docs/features/hetzner" },
  { id: "bitqate", name: "BitQate", href: "/docs/features/bitqate" },
  { id: "railway", name: "Railway", href: "/docs/features/railway" },
  { id: "fly", name: "Fly.io", href: "/docs/features/fly" },
  { id: "digitalocean", name: "DigitalOcean", href: "/docs/features/digitalocean" },
  { id: "boxd", name: "boxd", href: "/docs/features/boxd" },
] as const;

function ProviderMark({ id }: { id: (typeof providers)[number]["id"] }) {
  const mark = marks[id];
  return (
    <svg
      viewBox={mark.viewBox}
      className="size-11"
      aria-hidden
      fill="currentColor"
    >
      <path d={mark.d} fillRule={mark.rule} transform={mark.transform} />
    </svg>
  );
}

const pageSize = 6;
const pageCount = Math.ceil(providers.length / pageSize);
const lifts = [6, 11, 15, 15, 11, 6];

type Provider = (typeof providers)[number];

function pageAt(page: number): Provider[] {
  const start = (page * pageSize) % providers.length;
  return Array.from(
    { length: pageSize },
    (_, index) => providers[(start + index) % providers.length]!,
  );
}

function MarkFace({ provider, index }: { provider: Provider; index: number }) {
  const reduce = useReducedMotion();
  const controls = useAnimationControls();
  const [shown, setShown] = useState<Provider>(provider);
  const shownId = useRef(provider?.id);
  const run = useRef(0);

  useEffect(() => {
    if (shownId.current === provider.id) return;
    const token = ++run.current;
    const timer = window.setTimeout(() => {
      if (reduce) {
        if (run.current !== token) return;
        shownId.current = provider.id;
        setShown(provider);
        return;
      }
      void (async () => {
        try {
          await controls.start({
            rotateY: 90,
            y: -(lifts[index] ?? 8),
            transition: { duration: 0.26, ease: [0.45, 0, 0.9, 0.55] },
          });
          if (run.current !== token) return;
          shownId.current = provider.id;
          setShown(provider);
          controls.set({ rotateY: -90, y: -4 });
          await controls.start({
            rotateY: 0,
            y: 0,
            transition: { type: "spring", bounce: 0.16, duration: 0.58 },
          });
        } catch {
          // A newer flip or unmount stopped this one.
        }
      })();
    }, reduce ? 0 : index * 90);
    return () => {
      run.current += 1;
      window.clearTimeout(timer);
      controls.stop();
    };
  }, [controls, index, provider, reduce]);

  return (
    <div className="flex h-14 items-center justify-center [perspective:900px]">
      <motion.div
        animate={controls}
        initial={{ rotateY: 0, y: 0 }}
        className="[transform-style:preserve-3d]"
      >
        <Link
          href={shown.href}
          aria-label={shown.name}
          className="flex h-11 w-16 items-center justify-center text-[#9aa8bc] outline-none transition-colors hover:text-[#e8eef6] focus-visible:text-[#e8eef6] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-[#c6b6f2]"
        >
          <ProviderMark id={shown.id} />
        </Link>
      </motion.div>
    </div>
  );
}

function ProviderReel() {
  const reduce = useReducedMotion();
  const [page, setPage] = useState(0);
  const paused = useRef(false);

  useEffect(() => {
    if (reduce) return;
    const id = window.setInterval(() => {
      if (paused.current || document.hidden) return;
      setPage((current) => (current + 1) % pageCount);
    }, 4600);
    return () => window.clearInterval(id);
  }, [reduce]);

  if (reduce) {
    return (
      <ul className="grid grid-cols-3 items-center gap-x-8 gap-y-8 sm:grid-cols-6">
        {providers.map((provider) => (
          <li key={provider.id} className="flex justify-center">
            <Link
              href={provider.href}
              aria-label={provider.name}
              className="flex h-10 w-12 items-center justify-center text-[#9aa8bc] outline-none transition-colors hover:text-[#e8eef6] focus-visible:text-[#e8eef6] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-[#c6b6f2]"
            >
              <ProviderMark id={provider.id} />
            </Link>
          </li>
        ))}
      </ul>
    );
  }

  const visible = pageAt(page);

  return (
    <ul
      className="mx-auto flex w-full max-w-[72rem] items-center justify-between"
      onMouseEnter={() => {
        paused.current = true;
      }}
      onMouseLeave={() => {
        paused.current = false;
      }}
      onFocusCapture={() => {
        paused.current = true;
      }}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
          paused.current = false;
        }
      }}
    >
      {visible.map((provider, index) => (
        <li key={index}>
          <MarkFace provider={provider} index={index} />
        </li>
      ))}
    </ul>
  );
}

export function ProviderSection() {
  return (
    <section aria-label="Cloud providers" className="relative bg-[#05080f] text-[#e8eef6]">
      <div
        aria-hidden
        className="pointer-events-none absolute inset-x-0 -top-20 h-20 bg-gradient-to-b from-transparent to-[#05080f]"
      />
      <div className="mx-auto w-full max-w-[90rem] px-4 py-16 sm:px-6 sm:py-20 xl:px-10">
        <ProviderReel />
      </div>
    </section>
  );
}
