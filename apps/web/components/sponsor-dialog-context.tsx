"use client";

import dynamic from "next/dynamic";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";

import {
  isCheckoutSessionId,
  SPONSOR_COMPLETE_PARAM,
  SPONSOR_COMPLETE_VALUE,
  SPONSOR_SESSION_PARAM,
} from "@/lib/checkout-return";

const SponsorDialog = dynamic(
  () =>
    import("@/components/sponsor-dialog").then((mod) => ({
      default: mod.SponsorDialog,
    })),
  { ssr: false },
);

type SponsorReturnSnapshot = {
  sessionId: string | null;
  open: boolean;
};

const closedReturn: SponsorReturnSnapshot = { sessionId: null, open: false };
const returnListeners = new Set<() => void>();
let returnSnapshot: SponsorReturnSnapshot = closedReturn;
let returnRead = false;

function emitSponsorReturn() {
  for (const listener of returnListeners) listener();
}

function readSponsorReturnFromUrl(): SponsorReturnSnapshot {
  if (returnRead) return returnSnapshot;
  returnRead = true;
  const params = new URLSearchParams(window.location.search);
  if (params.get(SPONSOR_COMPLETE_PARAM) !== SPONSOR_COMPLETE_VALUE) {
    returnSnapshot = closedReturn;
    return returnSnapshot;
  }
  const rawId = params.get(SPONSOR_SESSION_PARAM);
  returnSnapshot = {
    sessionId: rawId && isCheckoutSessionId(rawId) ? rawId : null,
    open: true,
  };
  return returnSnapshot;
}

function getServerSponsorReturn(): SponsorReturnSnapshot {
  return closedReturn;
}

function subscribeSponsorReturn(listener: () => void) {
  returnListeners.add(listener);
  return () => {
    returnListeners.delete(listener);
  };
}

function dismissSponsorReturn() {
  if (returnSnapshot === closedReturn) return;
  returnSnapshot = closedReturn;
  emitSponsorReturn();
}

function stripSponsorReturnQuery() {
  const params = new URLSearchParams(window.location.search);
  if (params.get(SPONSOR_COMPLETE_PARAM) !== SPONSOR_COMPLETE_VALUE) return;
  params.delete(SPONSOR_COMPLETE_PARAM);
  params.delete(SPONSOR_SESSION_PARAM);
  const search = params.toString();
  const next = `${window.location.pathname}${search ? `?${search}` : ""}${window.location.hash}`;
  window.history.replaceState(null, "", next);
}

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
  const returnFlow = useSyncExternalStore(
    subscribeSponsorReturn,
    readSponsorReturnFromUrl,
    getServerSponsorReturn,
  );
  const [userOpen, setUserOpen] = useState(false);
  const [mounted, setMounted] = useState(false);
  const open = userOpen || returnFlow.open;
  if (open && !mounted) {
    setMounted(true);
  }
  const openDialog = useCallback(() => {
    setUserOpen(true);
  }, []);
  const closeDialog = useCallback(() => {
    setUserOpen(false);
    dismissSponsorReturn();
  }, []);

  useEffect(() => {
    if (!returnFlow.open) return;
    stripSponsorReturnQuery();
  }, [returnFlow.open]);

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
          resumeSessionId={returnFlow.sessionId}
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
