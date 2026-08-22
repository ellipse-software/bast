import { SponsorCta } from "@/components/sponsor-dialog-context";
import { TestimonialAvatar } from "@/components/testimonial-avatar";
import { formatUsd, sponsors } from "@/lib/sponsors";
import { splitTweetText } from "@/lib/tweet-text";
import { parseXProfileUrl } from "@/lib/x-handle";

export async function Sponsors() {
  const parsed = sponsors
    .map((sponsor) => {
      const username = parseXProfileUrl(sponsor.href);
      return username ? { ...sponsor, username } : null;
    })
    .filter((sponsor) => sponsor !== null)
    .sort((a, b) => b.amount - a.amount);

  return (
    <section className="w-full">
      <div className="mb-10 flex flex-col items-center text-center">
        <h2 className="text-3xl font-medium tracking-tight">Sponsors</h2>
        <p className="mt-2 max-w-lg text-sm leading-relaxed text-muted">
          Bast is free and open source. Sponsorships fund development.{" "}
          <SponsorCta />
        </p>
      </div>
      <div
        className={
          parsed.length === 1
            ? "flex justify-center"
            : "grid grid-cols-1 justify-items-center gap-x-8 gap-y-12 md:grid-cols-3"
        }
      >
        {parsed.map((sponsor) => {
          const itemClass =
            "flex w-full max-w-md flex-col items-center text-center";
          const inner = (
            <>
              <p className="text-sm tabular-nums text-foreground">
                {formatUsd(sponsor.amount)}
              </p>
              {sponsor.message ? (
                <p className="mt-3 max-w-md text-base leading-relaxed text-foreground/90">
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
                    className="flex size-6 shrink-0 items-center justify-center rounded-full bg-border text-[0.55rem] font-medium tracking-wide text-muted"
                    aria-hidden
                  >
                    ?
                  </span>
                ) : (
                  <TestimonialAvatar
                    name={sponsor.username}
                    username={sponsor.username}
                    size="sm"
                  />
                )}
                <span className="text-xs text-muted group-hover:text-foreground group-focus-visible:text-foreground">
                  {sponsor.anonymous ? "Anonymous" : `@${sponsor.username}`}
                </span>
              </div>
            </>
          );

          if (sponsor.anonymous) {
            return (
              <div
                key={`${sponsor.username}-${sponsor.amount}`}
                className={itemClass}
              >
                {inner}
              </div>
            );
          }

          return (
            <a
              key={sponsor.username}
              href={sponsor.href}
              target="_blank"
              rel="noopener noreferrer"
              className={`group ${itemClass} outline-none`}
            >
              {inner}
            </a>
          );
        })}
      </div>
    </section>
  );
}
