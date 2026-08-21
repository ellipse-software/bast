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

function InitialsMark({ name }: { name: string }) {
  return (
    <span
      className="flex size-10 items-center justify-center rounded-full bg-border text-[0.7rem] font-medium tracking-wide text-foreground"
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
}: {
  name: string;
  username: string;
  src?: string;
}) {
  const [failed, setFailed] = useState(false);
  const handle = normalizeXHandle(username);
  const url = src ?? (handle ? `/api/avatar/${handle}` : null);

  return (
    <span className="relative size-10 shrink-0">
      <InitialsMark name={name} />
      {url && !failed ? (
        // Dynamic avatar endpoint / optional override; 40px.
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={url}
          alt=""
          width={40}
          height={40}
          className="absolute inset-0 size-10 rounded-full object-cover"
          onError={() => setFailed(true)}
        />
      ) : null}
    </span>
  );
}
