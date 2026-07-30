import * as Sentry from "@sentry/node";

import type { ErrorReportPayload } from "@/lib/errors";

let initialized = false;

function ensureSentry(): boolean {
  const dsn = process.env.SENTRY_DSN;
  if (!dsn) {
    return false;
  }

  if (!initialized) {
    Sentry.init({
      dsn,
      defaultIntegrations: false,
      tracesSampleRate: 0,
    });
    initialized = true;
  }

  return true;
}

export async function captureCliError(
  payload: ErrorReportPayload,
  country?: string,
) {
  if (!ensureSentry()) {
    console.info("[error-report]", { ...payload, country });
    return;
  }

  const error = new Error(payload.message);
  error.name = payload.code ? `BastError:${payload.code}` : "BastError";
  if (payload.stack) {
    error.stack = payload.stack;
  }

  Sentry.withScope((scope) => {
    scope.setLevel("error");
    scope.setTag("source", payload.source);
    scope.setTag("os", payload.os);
    scope.setTag("arch", payload.arch);
    scope.setTag("version", payload.version);
    if (payload.context) {
      scope.setTag("context", payload.context);
    }
    if (payload.code) {
      scope.setTag("code", payload.code);
    }
    if (country) {
      scope.setTag("country", country);
    }
    if (payload.command) {
      scope.setExtra("command", payload.command);
    }
    scope.setExtra("bast_version", payload.version);
    Sentry.captureException(error);
  });

  await Sentry.flush(2000);
}
