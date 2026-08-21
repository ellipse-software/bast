"use client";

import {
  EmbeddedCheckout,
  EmbeddedCheckoutProvider,
} from "@stripe/react-stripe-js";
import { loadStripe, type Stripe } from "@stripe/stripe-js";
import { useCallback, useEffect, useId, useRef, useState } from "react";

import { startSponsorCheckout } from "@/app/actions/stripe";
import {
  formatUsd,
  SPONSOR_MAX_USD,
  SPONSOR_MIN_USD,
  SPONSOR_MESSAGE_MAX,
  SPONSOR_PRESETS_USD,
} from "@/lib/sponsors";

type Step = "amount" | "pay" | "done";

const stripePromise: Promise<Stripe | null> = process.env
  .NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY
  ? loadStripe(process.env.NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY)
  : Promise.resolve(null);

export function SponsorDialog({
  open,
  paymentsEnabled,
  onClose,
}: {
  open: boolean;
  paymentsEnabled: boolean;
  onClose: () => void;
}) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const [step, setStep] = useState<Step>("amount");
  const [preset, setPreset] = useState<number | "custom">(25);
  const [customUsd, setCustomUsd] = useState("25");
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
      setStep("amount");
      setError(null);
      setAnonymous(false);
      dialog.showModal();
    }
    if (!open && dialog.open) {
      dialog.close();
    }
  }, [open]);

  const fetchClientSecret = useCallback(async () => {
    const result = await startSponsorCheckout({
      amountUsd,
      handle,
      message,
      anonymous,
    });
    if ("error" in result) {
      setError(result.error);
      setStep("amount");
      throw new Error(result.error);
    }
    return result.clientSecret;
  }, [amountUsd, handle, message, anonymous]);

  return (
    <dialog
      ref={dialogRef}
      aria-labelledby={titleId}
      className="m-auto w-[calc(100%-2rem)] max-w-md border border-border bg-background p-0 text-foreground backdrop:bg-black/70"
    >
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
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

          <button
            type="submit"
            className="border border-accent bg-[color-mix(in_srgb,var(--accent)_18%,var(--background))] px-3 py-2 text-sm font-medium text-foreground hover:bg-[color-mix(in_srgb,var(--accent)_28%,var(--background))]"
          >
            Continue
          </button>
        </form>
      ) : null}

      {step === "pay" ? (
        <div className="p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <p className="text-sm tabular-nums">{formatUsd(amountUsd)}</p>
            <button
              type="button"
              onClick={() => {
                setStep("amount");
                setError(null);
              }}
              className="text-sm text-muted hover:text-foreground"
            >
              Back
            </button>
          </div>
          <EmbeddedCheckoutProvider
            key={`${amountUsd}-${handle}-${message}-${anonymous}`}
            stripe={stripePromise}
            options={{
              fetchClientSecret,
              onComplete: () => setStep("done"),
            }}
          >
            <EmbeddedCheckout />
          </EmbeddedCheckoutProvider>
        </div>
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
