import { SponsorCta } from "@/components/sponsor-dialog-context";
import { TestimonialAvatar } from "@/components/testimonial-avatar";
import {
  formatUsd,
  partitionSponsorLayout,
  resolveSponsors,
  sponsors,
  type ResolvedSponsor,
} from "@/lib/sponsors";
import { splitTweetText } from "@/lib/tweet-text";

function featuredRowClass(count: number): string {
  if (count <= 1) {
    return "flex justify-center";
  }
  return "mx-auto grid w-full max-w-3xl grid-cols-1 justify-items-center gap-x-8 gap-y-12 md:grid-cols-2";
}

function gridRowClass(): string {
  return "grid grid-cols-1 justify-items-center gap-x-8 gap-y-12 md:grid-cols-3";
}

function SponsorItem({
  sponsor,
  featured = false,
}: {
  sponsor: ResolvedSponsor;
  featured?: boolean;
}) {
  const itemClass = featured
    ? "flex w-full max-w-xl flex-col items-center text-center"
    : "flex w-full max-w-md flex-col items-center text-center";
  const inner = (
    <>
      <p
        className={`tabular-nums text-foreground ${featured ? "text-base" : "text-sm"}`}
      >
        {formatUsd(sponsor.amount)}
      </p>
      {sponsor.message ? (
        <p
          className={`mt-3 leading-relaxed text-foreground/90 ${featured ? "max-w-xl text-lg" : "max-w-md text-base"}`}
        >
          {splitTweetText(sponsor.message).map((part, index) =>
            part.type === "mention" ? (
              <span key={index} className="text-accent">
                {part.value}
              </span>
            ) : (
              part.value
            ),
          )}
        </p>
      ) : null}
      <div className="mt-4 flex items-center justify-center gap-1.5">
        {sponsor.anonymous ? (
          <span
            className={`flex shrink-0 items-center justify-center rounded-full bg-border font-medium tracking-wide text-muted ${featured ? "size-10 text-[0.7rem]" : "size-6 text-[0.55rem]"}`}
            aria-hidden
          >
            ?
          </span>
        ) : (
          <TestimonialAvatar
            name={sponsor.name}
            username={sponsor.username ?? sponsor.name}
            src={sponsor.avatar}
            size={featured ? "md" : "sm"}
          />
        )}
        <span
          className={`text-muted group-hover:text-foreground group-focus-visible:text-foreground ${featured ? "text-sm" : "text-xs"}`}
        >
          {sponsor.anonymous
            ? "Anonymous"
            : sponsor.username
              ? `@${sponsor.username}`
              : sponsor.name}
        </span>
      </div>
    </>
  );

  if (sponsor.anonymous) {
    return (
      <div className={itemClass}>
        {inner}
      </div>
    );
  }

  return (
    <a
      href={sponsor.href}
      target="_blank"
      rel="noopener noreferrer"
      className={`group ${itemClass} outline-none`}
    >
      {inner}
    </a>
  );
}

export async function Sponsors() {
  const parsed = resolveSponsors(sponsors);
  const { featured, rest } = partitionSponsorLayout(parsed);

  return (
    <section className="w-full">
      <div className="mb-10 flex flex-col items-center text-center">
        <h2 className="text-3xl font-medium tracking-tight">Sponsors</h2>
        <p className="mt-2 max-w-lg text-sm leading-relaxed text-muted">
          Bast is free and open source. Sponsorships fund development.{" "}
          <SponsorCta />
        </p>
      </div>
      <div className="flex flex-col gap-16">
        {featured.length > 0 ? (
          <div className={featuredRowClass(featured.length)}>
            {featured.map((sponsor) => (
              <SponsorItem
                key={sponsor.anonymous ? `${sponsor.name}-${sponsor.amount}` : sponsor.href}
                sponsor={sponsor}
                featured
              />
            ))}
          </div>
        ) : null}
        {rest.length > 0 ? (
          <div className={gridRowClass()}>
            {rest.map((sponsor) => (
              <SponsorItem
                key={sponsor.anonymous ? `${sponsor.name}-${sponsor.amount}` : sponsor.href}
                sponsor={sponsor}
              />
            ))}
          </div>
        ) : null}
      </div>
    </section>
  );
}
