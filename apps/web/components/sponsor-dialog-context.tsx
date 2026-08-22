"use client";

import dynamic from "next/dynamic";
import {
  createContext,
  useCallback,
  useContext,
  useState,
  type ReactNode,
} from "react";

const SponsorDialog = dynamic(
  () =>
    import("@/components/sponsor-dialog").then((mod) => ({
      default: mod.SponsorDialog,
    })),
  { ssr: false },
);

type SponsorDialogContextValue = {
  open: () => void;
  close: () => void;
};

const SponsorDialogContext = createContext<SponsorDialogContextValue | null>(
  null,
);

export function SponsorDialogProvider({
  children,
  paymentsEnabled,
}: {
  children: ReactNode;
  paymentsEnabled: boolean;
}) {
  const [mounted, setMounted] = useState(false);
  const [open, setOpen] = useState(false);
  const openDialog = useCallback(() => {
    setMounted(true);
    setOpen(true);
  }, []);
  const closeDialog = useCallback(() => setOpen(false), []);

  return (
    <SponsorDialogContext.Provider
      value={{ open: openDialog, close: closeDialog }}
    >
      {children}
      {mounted ? (
        <SponsorDialog
          open={open}
          paymentsEnabled={paymentsEnabled}
          onClose={closeDialog}
        />
      ) : null}
    </SponsorDialogContext.Provider>
  );
}

export function useSponsorDialog(): SponsorDialogContextValue {
  const value = useContext(SponsorDialogContext);
  if (!value) {
    throw new Error("useSponsorDialog requires SponsorDialogProvider");
  }
  return value;
}

export function SponsorCta() {
  const { open } = useSponsorDialog();

  return (
    <button
      type="button"
      onClick={open}
      className="inline text-foreground transition-colors hover:text-accent focus-visible:text-accent"
    >
      Sponsor now
    </button>
  );
}

export function SponsorNavButton({
  className,
  onClick,
}: {
  className: string;
  onClick?: () => void;
}) {
  const { open } = useSponsorDialog();

  return (
    <button
      type="button"
      className={className}
      onClick={() => {
        onClick?.();
        open();
      }}
    >
      Sponsor
    </button>
  );
}
