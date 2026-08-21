import type { CSSProperties } from "react";
import { connection } from "next/server";

import { TestimonialAvatar } from "@/components/testimonial-avatar";
import { testimonials } from "@/lib/testimonials";
import { shuffle, splitTweetText } from "@/lib/tweet-text";
import { getXTweets, type XTweet } from "@/lib/x-tweet";

const ROW_COUNT = 3;
const MIN_CARDS_PER_ROW = 8;
const ROW_MOTION = [
  { duration: "56s", delay: "-18s", reverse: false },
  { duration: "74s", delay: "-40s", reverse: true },
  { duration: "64s", delay: "-8s", reverse: false },
] as const;

function handleOf(username: string): string {
  return username.startsWith("@") ? username : `@${username}`;
}

type ResolvedTestimonial = {
  href: string;
  username: string;
  message: string;
  name: string;
  verified: boolean;
};

function toCard(tweet: XTweet): ResolvedTestimonial {
  return {
    href: tweet.url,
    username: tweet.username,
    message: tweet.text,
    name: tweet.profile.name,
    verified: tweet.profile.verified,
  };
}

function splitRows<T>(items: readonly T[]): T[][] {
  const rows: T[][] = Array.from({ length: ROW_COUNT }, () => []);
  items.forEach((item, index) => {
    rows[index % ROW_COUNT]?.push(item);
  });
  return rows.filter((row) => row.length > 0);
}

function fillRow<T>(items: T[]): T[] {
  if (items.length === 0) return items;
  const copies = Math.max(2, Math.ceil(MIN_CARDS_PER_ROW / items.length));
  return Array.from({ length: copies }, () => items).flat();
}

function VerifiedBadge() {
  return (
    <svg
      viewBox="0 0 22 22"
      className="size-[18px] shrink-0 text-foreground"
      role="img"
      aria-label="Verified"
    >
      <path
        fill="currentColor"
        d="M20.396 11c-.018-.646-.215-1.275-.57-1.816-.354-.54-.852-.972-1.438-1.246.223-.607.27-1.264.14-1.897-.13-.634-.437-1.218-.882-1.687-.47-.445-1.053-.75-1.687-.882-.633-.13-1.29-.083-1.897.14-.273-.587-.704-1.086-1.245-1.44S11.647 1.62 11 1.604c-.646.017-1.273.213-1.813.568s-.969.854-1.24 1.44c-.608-.223-1.267-.272-1.902-.14-.635.13-1.22.436-1.69.882-.445.47-.749 1.055-.878 1.688-.13.633-.08 1.29.144 1.896-.587.274-1.087.705-1.443 1.245-.356.54-.555 1.17-.574 1.817.02.647.218 1.276.574 1.817.356.54.856.972 1.443 1.245-.224.606-.274 1.263-.144 1.896.13.634.434 1.218.878 1.688.47.443 1.054.747 1.687.878.633.132 1.29.084 1.897-.136.274.586.705 1.084 1.246 1.439.54.354 1.17.551 1.816.569.647-.016 1.276-.213 1.817-.567s.972-.854 1.245-1.44c.604.239 1.266.296 1.903.164.636-.132 1.22-.447 1.68-.907.46-.46.776-1.044.908-1.681s.075-1.299-.165-1.903c.586-.274 1.084-.705 1.439-1.246.354-.54.551-1.17.569-1.816zM9.662 14.85l-3.429-3.428 1.293-1.302 2.072 2.072 4.4-4.794 1.347 1.246z"
      />
    </svg>
  );
}

function XMark() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="mt-0.5 size-[18px] shrink-0 text-muted"
      aria-hidden
    >
      <path
        fill="currentColor"
        d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-4.714-6.231-5.401 6.231H2.74l7.995-9.232L1.254 2.25H8.08l4.253 5.622L18.244 2.25zm-1.161 17.52h1.833L7.084 4.126H5.117z"
      />
    </svg>
  );
}

function TestimonialCard({
  testimonial,
  interactive,
}: {
  testimonial: ResolvedTestimonial;
  interactive: boolean;
}) {
  const handle = handleOf(testimonial.username);

  return (
    <a
      href={testimonial.href}
      className="flex h-[9rem] w-[22rem] shrink-0 gap-3 border border-border bg-background px-4 py-3 text-left outline-none transition-[background-color,border-color] duration-150 hover:border-accent hover:bg-[color-mix(in_srgb,var(--accent)_9%,var(--background))] focus-visible:border-accent focus-visible:bg-[color-mix(in_srgb,var(--accent)_9%,var(--background))]"
      target="_blank"
      rel="noopener noreferrer"
      tabIndex={interactive ? undefined : -1}
    >
      <TestimonialAvatar
        name={testimonial.name}
        username={testimonial.username}
      />
      <div className="min-w-0 flex-1">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-0.5 text-[15px] leading-5">
              <span className="truncate font-bold text-foreground">
                {testimonial.name}
              </span>
              {testimonial.verified ? <VerifiedBadge /> : null}
            </div>
            <p className="truncate text-[15px] leading-5 text-muted">{handle}</p>
          </div>
          <XMark />
        </div>
        <p className="mt-3 line-clamp-3 whitespace-pre-wrap text-[15px] leading-5 text-foreground">
          {splitTweetText(testimonial.message).map((part, index) =>
            part.type === "mention" ? (
              <span key={index} className="text-accent">
                {part.value}
              </span>
            ) : (
              part.value
            ),
          )}
        </p>
      </div>
    </a>
  );
}

function TestimonialTrack({
  items,
  duration,
  delay,
  reverse,
}: {
  items: ResolvedTestimonial[];
  duration: string;
  delay: string;
  reverse: boolean;
}) {
  const filled = fillRow(items);
  const motionStyle: CSSProperties = {
    animationDuration: duration,
    animationDelay: delay,
    animationDirection: reverse ? "reverse" : "normal",
  };

  return (
    <div
      className="flex w-max animate-testimonial-marquee group-hover/testimonials:paused group-focus-within/testimonials:paused motion-reduce:animate-none"
      style={motionStyle}
    >
      <div className="flex items-stretch gap-3 pr-3">
        {filled.map((testimonial, index) => (
          <TestimonialCard
            key={`a-${testimonial.username}-${index}`}
            testimonial={testimonial}
            interactive
          />
        ))}
      </div>
      <div className="flex items-stretch gap-3 pr-3 motion-reduce:hidden" aria-hidden inert>
        {filled.map((testimonial, index) => (
          <TestimonialCard
            key={`b-${testimonial.username}-${index}`}
            testimonial={testimonial}
            interactive={false}
          />
        ))}
      </div>
    </div>
  );
}

export async function Testimonials() {
  if (testimonials.length === 0) return null;

  await connection();
  const resolved = shuffle((await getXTweets(testimonials)).map(toCard));
  if (resolved.length === 0) return null;

  const rows = splitRows(resolved);

  return (
    <section
      aria-label="Testimonials"
      className="group/testimonials relative w-full overflow-hidden"
    >
      <div className="flex flex-col gap-3">
        {rows.map((row, index) => {
          const motion = ROW_MOTION[index] ?? ROW_MOTION[0];
          return (
            <TestimonialTrack
              key={index}
              items={row}
              duration={motion.duration}
              delay={motion.delay}
              reverse={motion.reverse}
            />
          );
        })}
      </div>
      <div
        aria-hidden
        className="pointer-events-none absolute inset-y-0 left-0 z-10 bg-gradient-to-r from-background from-40% via-background/70 to-transparent"
        style={{
          width:
            "min(16rem, max(5rem, calc((100% - 64rem) / 2 + 3.5rem)))",
        }}
      />
      <div
        aria-hidden
        className="pointer-events-none absolute inset-y-0 right-0 z-10 bg-gradient-to-l from-background from-40% via-background/70 to-transparent"
        style={{
          width:
            "min(16rem, max(5rem, calc((100% - 64rem) / 2 + 3.5rem)))",
        }}
      />
    </section>
  );
}
