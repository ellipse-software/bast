"use client";

import { useState } from "react";

import { normalizeXHandle } from "@/lib/x-handle";

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  const first = parts[0][0];
  const last = parts[parts.length - 1][0];
  return `${first ?? ""}${last ?? ""}`.toUpperCase();
}

const avatarSize = {
  md: {
    box: "size-10",
    text: "text-[0.7rem]",
    px: 40,
  },
  sm: {
    box: "size-6",
    text: "text-[0.55rem]",
    px: 24,
  },
} as const;

function InitialsMark({
  name,
  size,
}: {
  name: string;
  size: keyof typeof avatarSize;
}) {
  const token = avatarSize[size];
  return (
    <span
      className={`flex ${token.box} items-center justify-center rounded-full bg-border ${token.text} font-medium tracking-wide text-foreground`}
      aria-hidden
    >
      {initials(name)}
    </span>
  );
}

export function TestimonialAvatar({
  name,
  username,
  src,
  size = "md",
}: {
  name: string;
  username: string;
  src?: string;
  size?: keyof typeof avatarSize;
}) {
  const [failed, setFailed] = useState(false);
  const handle = normalizeXHandle(username);
  const url = src ?? (handle ? `/api/avatar/${handle}` : null);
  const token = avatarSize[size];

  return (
    <span className={`relative ${token.box} shrink-0`}>
      <InitialsMark name={name} size={size} />
      {url && !failed ? (
        // Dynamic avatar endpoint / optional override.
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={url}
          alt=""
          width={token.px}
          height={token.px}
          className={`absolute inset-0 ${token.box} rounded-full object-cover`}
          onError={() => setFailed(true)}
        />
      ) : null}
    </span>
  );
}
