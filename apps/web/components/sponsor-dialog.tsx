"use client";

import {
  CheckoutElementsProvider,
  ContactDetailsElement,
  CurrencySelectorElement,
  PaymentElement,
  useCheckoutElements,
} from "@stripe/react-stripe-js/checkout";
import { loadStripe, type Stripe } from "@stripe/stripe-js";
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";

import {
  getSponsorCheckoutStatus,
  startSponsorCheckout,
} from "@/app/actions/stripe";
import {
  formatUsd,
  SPONSOR_MAX_USD,
  SPONSOR_MIN_USD,
  SPONSOR_MESSAGE_MAX,
  SPONSOR_PRESETS_USD,
  type SponsorInterval,
} from "@/lib/sponsors";
import { sponsorCheckoutAppearance } from "@/lib/stripe-appearance";

type Step = "amount" | "pay" | "verify" | "done";

const stripePromise: Promise<Stripe | null> = process.env
  .NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY
  ? loadStripe(process.env.NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY)
  : Promise.resolve(null);

const PAYMENT_ELEMENT_OPTIONS = {
  layout: {
    type: "accordion" as const,
    defaultCollapsed: false,
    spacedAccordionItems: true,
  },
};

const primaryButtonClass =
  "border border-accent bg-[color-mix(in_srgb,var(--accent)_18%,var(--background))] px-3 py-2 text-sm font-medium text-foreground hover:bg-[color-mix(in_srgb,var(--accent)_28%,var(--background))] disabled:opacity-50";

export function SponsorDialog({
  open,
  paymentsEnabled,
  onClose,
  resumeSessionId,
}: {
  open: boolean;
  paymentsEnabled: boolean;
  onClose: () => void;
  resumeSessionId: string | null;
}) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const [step, setStep] = useState<Step>("amount");
  const [preset, setPreset] = useState<number | "custom">(25);
  const [customUsd, setCustomUsd] = useState("25");
  const [interval, setInterval] = useState<SponsorInterval>("once");
  const [handle, setHandle] = useState("");
  const [message, setMessage] = useState("");
  const [anonymous, setAnonymous] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const amountUsd = preset === "custom" ? Number(customUsd) : preset;

  function close() {
    dialogRef.current?.close();
  }

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    function handleClose() {
      onClose();
    }
    dialog.addEventListener("close", handleClose);
    return () => dialog.removeEventListener("close", handleClose);
  }, [onClose]);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    if (open && !dialog.open) {
      setError(null);
      setAnonymous(false);
      setInterval("once");
      setStep(resumeSessionId ? "verify" : "amount");
      dialog.showModal();
    }
    if (!open && dialog.open) {
      dialog.close();
    }
  }, [open, resumeSessionId]);

  useEffect(() => {
    if (!open || step !== "verify" || !resumeSessionId) return;
    let cancelled = false;
    getSponsorCheckoutStatus(resumeSessionId).then((result) => {
      if (cancelled) return;
      if ("status" in result && result.status === "complete") {
        setStep("done");
        return;
      }
      setError(
        "error" in result ? result.error : "Payment was not completed.",
      );
      setStep("amount");
    });
    return () => {
      cancelled = true;
    };
  }, [open, step, resumeSessionId]);

  const fetchClientSecret = useCallback(async () => {
    const result = await startSponsorCheckout({
      amountUsd,
      interval,
      handle,
      message,
      anonymous,
      returnPath: window.location.pathname,
    });
    if ("error" in result) {
      setError(result.error);
      setStep("amount");
      throw new Error(result.error);
    }
    return result.clientSecret;
  }, [amountUsd, interval, handle, message, anonymous]);

  return (
    <dialog
      ref={dialogRef}
      aria-labelledby={titleId}
      className="m-auto max-h-[min(90dvh,48rem)] w-[calc(100%-2rem)] max-w-md overflow-y-auto border border-border bg-background p-0 text-foreground backdrop:bg-black/70"
    >
      <div className="sticky top-0 z-10 flex items-center justify-between border-b border-border bg-background px-4 py-3">
        <h3 id={titleId} className="text-sm font-medium tracking-tight">
          Sponsor
        </h3>
        <button
          type="button"
          onClick={close}
          className="text-sm text-muted hover:text-foreground"
        >
          Close
        </button>
      </div>

      {step === "amount" ? (
        <form
          className="flex flex-col gap-4 p-4"
          onSubmit={(event) => {
            event.preventDefault();
            if (!paymentsEnabled) {
              setError("Payments are not configured yet.");
              return;
            }
            if (
              !Number.isFinite(amountUsd) ||
              amountUsd < SPONSOR_MIN_USD ||
              amountUsd > SPONSOR_MAX_USD
            ) {
              setError(
                `Enter an amount between ${formatUsd(SPONSOR_MIN_USD)} and ${formatUsd(SPONSOR_MAX_USD)}.`,
              );
              return;
            }
            setError(null);
            setStep("pay");
          }}
        >
          <IntervalSwitch value={interval} onChange={setInterval} />

          <fieldset>
            <legend className="mb-2 text-sm text-muted">Amount</legend>
            <div className="flex flex-wrap gap-2">
              {SPONSOR_PRESETS_USD.map((value) => {
                const selected = preset === value;
                return (
                  <button
                    key={value}
                    type="button"
                    onClick={() => {
                      setPreset(value);
                      setCustomUsd(String(value));
                    }}
                    className={`border px-3 py-1.5 text-sm tabular-nums ${
                      selected
                        ? "border-accent bg-[color-mix(in_srgb,var(--accent)_9%,var(--background))] text-foreground"
                        : "border-border text-muted hover:text-foreground"
                    }`}
                  >
                    {formatUsd(value)}
                  </button>
                );
              })}
              <button
                type="button"
                onClick={() => setPreset("custom")}
                className={`border px-3 py-1.5 text-sm ${
                  preset === "custom"
                    ? "border-accent bg-[color-mix(in_srgb,var(--accent)_9%,var(--background))] text-foreground"
                    : "border-border text-muted hover:text-foreground"
                }`}
              >
                Custom
              </button>
            </div>
            {preset === "custom" ? (
              <label className="mt-3 flex items-center gap-2 border border-border bg-background px-3 py-2 text-sm">
                <span className="text-muted">$</span>
                <input
                  type="number"
                  min={SPONSOR_MIN_USD}
                  max={SPONSOR_MAX_USD}
                  step="1"
                  value={customUsd}
                  onChange={(event) => setCustomUsd(event.target.value)}
                  className="w-full bg-transparent text-foreground outline-none"
                />
              </label>
            ) : null}
          </fieldset>

          <label className="flex flex-col gap-1.5 text-sm">
            <span className="text-muted">X username</span>
            <input
              type="text"
              name="handle"
              autoComplete="off"
              spellCheck={false}
              placeholder="@handle"
              value={handle}
              onChange={(event) => setHandle(event.target.value)}
              className="border border-border bg-background px-3 py-2 text-foreground outline-none focus:border-accent"
            />
          </label>

          <label className="flex flex-col gap-1.5 text-sm">
            <span className="text-muted">Note</span>
            <textarea
              name="message"
              rows={3}
              maxLength={SPONSOR_MESSAGE_MAX}
              value={message}
              onChange={(event) => setMessage(event.target.value)}
              className="resize-none border border-border bg-background px-3 py-2 text-foreground outline-none focus:border-accent"
            />
          </label>

          <label className="flex cursor-pointer items-center gap-2 border border-border bg-background px-3 py-2 text-sm focus-within:border-accent">
            <input
              type="checkbox"
              name="anonymous"
              checked={anonymous}
              onChange={(event) => setAnonymous(event.target.checked)}
              className="sr-only"
            />
            <span
              aria-hidden
              className={`flex size-3.5 shrink-0 items-center justify-center border ${
                anonymous
                  ? "border-accent bg-[color-mix(in_srgb,var(--accent)_18%,var(--background))] text-foreground"
                  : "border-border text-transparent"
              }`}
            >
              <svg viewBox="0 0 12 12" className="size-2.5" fill="none">
                <path
                  d="M2.2 6.2 4.7 8.6 9.8 3.2"
                  stroke="currentColor"
                  strokeWidth="1.75"
                  strokeLinecap="square"
                />
              </svg>
            </span>
            Anonymous
          </label>

          {error ? <p className="text-sm text-red-400">{error}</p> : null}

          <button type="submit" className={primaryButtonClass}>
            Continue
          </button>
        </form>
      ) : null}

      {step === "verify" ? (
        <div className="p-4">
          <CheckoutSpinner />
        </div>
      ) : null}

      {step === "pay" ? (
        <SponsorPayStep
          key={`${amountUsd}-${interval}-${handle}-${message}-${anonymous}`}
          amountUsd={amountUsd}
          interval={interval}
          fetchClientSecret={fetchClientSecret}
          onBack={() => {
            setStep("amount");
            setError(null);
          }}
          onComplete={() => setStep("done")}
        />
      ) : null}

      {step === "done" ? (
        <div className="flex flex-col gap-4 p-4">
          <p className="text-sm leading-relaxed">
            Thank you for your sponsorship. We&apos;ll process this now, and
            I&apos;ll send you a quick message as a thank you.
          </p>
          <button
            type="button"
            onClick={close}
            className="border border-border px-3 py-2 text-sm hover:border-accent"
          >
            Close
          </button>
        </div>
      ) : null}
    </dialog>
  );
}

function SponsorPayStep({
  amountUsd,
  interval,
  fetchClientSecret,
  onBack,
  onComplete,
}: {
  amountUsd: number;
  interval: SponsorInterval;
  fetchClientSecret: () => Promise<string>;
  onBack: () => void;
  onComplete: () => void;
}) {
  const clientSecret = useMemo(
    () => fetchClientSecret(),
    [fetchClientSecret],
  );
  const options = useMemo(
    () => ({
      clientSecret,
      adaptivePricing: { allowed: true },
      elementsOptions: {
        appearance: sponsorCheckoutAppearance,
        loader: "auto" as const,
      },
    }),
    [clientSecret],
  );

  return (
    <div className="p-4">
      <CheckoutElementsProvider stripe={stripePromise} options={options}>
        <SponsorCheckoutForm
          amountUsd={amountUsd}
          interval={interval}
          onBack={onBack}
          onComplete={onComplete}
        />
      </CheckoutElementsProvider>
    </div>
  );
}

function SponsorCheckoutForm({
  amountUsd,
  interval,
  onBack,
  onComplete,
}: {
  amountUsd: number;
  interval: SponsorInterval;
  onBack: () => void;
  onComplete: () => void;
}) {
  const checkoutState = useCheckoutElements();
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [contactReady, setContactReady] = useState(false);
  const [paymentReady, setPaymentReady] = useState(false);
  const elementsReady = contactReady && paymentReady;

  if (checkoutState.type === "loading") {
    return (
      <div className="flex flex-col gap-4">
        <PayHeader
          amountLabel={formatSponsorPayAmount(formatUsd(amountUsd), interval)}
          onBack={onBack}
        />
        <CheckoutSpinner />
      </div>
    );
  }

  if (checkoutState.type === "error") {
    return (
      <>
        <PayHeader
          amountLabel={formatSponsorPayAmount(formatUsd(amountUsd), interval)}
          onBack={onBack}
        />
        <p className="text-sm text-red-400">{checkoutState.error.message}</p>
      </>
    );
  }

  const { checkout } = checkoutState;
  const amountLabel = formatSponsorPayAmount(
    checkout.total.total.amount || formatUsd(amountUsd),
    interval,
  );
  const showCurrencySelector = Boolean(checkout.currencyOptions?.length);

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={async (event) => {
        event.preventDefault();
        if (submitting || !checkout.canConfirm) return;
        setSubmitting(true);
        setMessage(null);
        try {
          const result = await checkout.confirm({ redirect: "if_required" });
          if (result.type === "error") {
            setMessage(result.error.message);
            setSubmitting(false);
            return;
          }
          onComplete();
        } catch (error) {
          setMessage(
            error instanceof Error
              ? error.message
              : "Could not complete payment.",
          );
          setSubmitting(false);
        }
      }}
    >
      <PayHeader amountLabel={amountLabel} onBack={onBack} />
      <div className="relative min-h-40">
        <div
          className={
            elementsReady
              ? "flex flex-col gap-4"
              : "pointer-events-none invisible flex flex-col gap-4"
          }
          aria-hidden={!elementsReady}
        >
          {showCurrencySelector ? <CurrencySelectorElement /> : null}
          <ContactDetailsElement
            onReady={() => setContactReady(true)}
            onLoadError={() => setContactReady(true)}
          />
          <PaymentElement
            options={PAYMENT_ELEMENT_OPTIONS}
            onReady={() => setPaymentReady(true)}
            onLoadError={() => setPaymentReady(true)}
          />
          {message ? <p className="text-sm text-red-400">{message}</p> : null}
          <button
            type="submit"
            disabled={submitting || !checkout.canConfirm}
            className={primaryButtonClass}
          >
            {interval === "month" ? "Sponsor monthly" : "Sponsor"}
          </button>
        </div>
        {elementsReady ? null : (
          <CheckoutSpinner className="absolute inset-0 flex items-center justify-center" />
        )}
      </div>
    </form>
  );
}

function CheckoutSpinner({ className }: { className?: string }) {
  return (
    <div
      className={className ?? "flex min-h-40 items-center justify-center"}
      aria-busy="true"
      aria-live="polite"
    >
      <span className="sr-only">Loading</span>
      <svg
        viewBox="0 0 16 16"
        className="size-4 animate-spin text-muted motion-reduce:animate-none"
        fill="none"
        aria-hidden
      >
        <circle
          cx="8"
          cy="8"
          r="5.5"
          stroke="currentColor"
          strokeOpacity="0.2"
          strokeWidth="1.5"
        />
        <path
          d="M13.5 8a5.5 5.5 0 0 0-5.5-5.5"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="square"
        />
      </svg>
    </div>
  );
}

function formatSponsorPayAmount(label: string, interval: SponsorInterval) {
  if (interval !== "month" || label.endsWith("/mo")) {
    return label;
  }
  return `${label}/mo`;
}

function IntervalSwitch({
  value,
  onChange,
}: {
  value: SponsorInterval;
  onChange: (next: SponsorInterval) => void;
}) {
  return (
    <div
      role="radiogroup"
      aria-label="Sponsorship frequency"
      className="relative border border-border p-0.5"
    >
      <div className="relative grid grid-cols-2">
        <span
          aria-hidden
          className={`pointer-events-none absolute inset-y-0 w-1/2 bg-highlight transition-transform duration-200 ease-out motion-reduce:transition-none ${
            value === "month" ? "translate-x-full" : "translate-x-0"
          }`}
        />
        {(
          [
            ["once", "One time"],
            ["month", "Monthly"],
          ] as const
        ).map(([id, label]) => {
          const selected = value === id;
          return (
            <button
              key={id}
              type="button"
              role="radio"
              aria-checked={selected}
              onClick={() => onChange(id)}
              className={`relative z-10 px-3 py-1.5 text-sm ${
                selected
                  ? "text-foreground"
                  : "text-muted hover:text-foreground"
              }`}
            >
              {label}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function PayHeader({
  amountLabel,
  onBack,
}: {
  amountLabel: string;
  onBack: () => void;
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <p className="text-sm tabular-nums">{amountLabel}</p>
      <button
        type="button"
        onClick={onBack}
        className="text-sm text-muted hover:text-foreground"
      >
        Back
      </button>
    </div>
  );
}
