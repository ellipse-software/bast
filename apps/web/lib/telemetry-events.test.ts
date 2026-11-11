import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

import { CLI_TELEMETRY_EVENTS } from "@/lib/telemetry";

const bastRoot = join(import.meta.dir, "../../bast");

function goFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      out.push(...goFiles(path));
      continue;
    }
    if (entry.endsWith(".go") && !entry.endsWith("_test.go")) {
      out.push(path);
    }
  }
  return out;
}

function trackArguments(source: string): string[] {
  const marker = "telemetry.Track(";
  const args: string[] = [];
  let index = 0;
  while (index < source.length) {
    const start = source.indexOf(marker, index);
    if (start < 0) {
      break;
    }
    let cursor = start + marker.length;
    let quote: string | null = null;
    const argStart = cursor;
    while (cursor < source.length) {
      const char = source[cursor];
      if (quote) {
        if (char === "\\") {
          cursor += 2;
          continue;
        }
        if (char === quote) {
          quote = null;
        }
        cursor += 1;
        continue;
      }
      if (char === '"' || char === "'" || char === "`") {
        quote = char;
        cursor += 1;
        continue;
      }
      if (char === "," || char === "\n") {
        args.push(source.slice(argStart, cursor).trim().replaceAll(/\s+/g, ""));
        break;
      }
      cursor += 1;
    }
    index = cursor;
  }
  return args;
}

function functionBody(source: string, signature: string): string {
  const start = source.indexOf(signature);
  if (start < 0) {
    throw new Error(`missing ${signature}`);
  }
  const next = source.indexOf("\nfunc ", start + signature.length);
  return next < 0 ? source.slice(start) : source.slice(start, next);
}

describe("CLI telemetry events", () => {
  const sources = goFiles(bastRoot).map((path) => ({
    path,
    text: readFileSync(path, "utf8"),
  }));
  const combined = sources.map(({ text }) => text).join("\n");

  test("allows every telemetry.Track string literal", () => {
    const literals = new Set<string>();
    const dynamics = new Set<string>();
    for (const arg of trackArguments(combined)) {
      if (/^"[^"]*"$/.test(arg)) {
        literals.add(arg.slice(1, -1));
      } else {
        dynamics.add(arg);
      }
    }

    const missing = [...literals].filter((event) => !CLI_TELEMETRY_EVENTS.has(event));
    expect(missing).toEqual([]);
    expect(dynamics).toEqual(
      new Set([
        "event",
        '"sync_"+msg.provider+"_fail"',
        '"sync_"+msg.provider+"_disable"',
        '"sync_"+msg.provider',
        '"sync_"+provider+"_disable"',
        'code+"_fail"',
        "code",
        "okEvent",
        "failEvent",
      ]),
    );
  });

  test("allows sync events for every provider the CLI can name", () => {
    const sync = readFileSync(join(bastRoot, "internal/cli/sync.go"), "utf8");
    const providers = [
      ...sync.matchAll(/provider != "([a-z0-9]+)"/g),
    ].map((match) => match[1]);
    expect(providers.length).toBe(12);

    const uiProviders = new Set<string>();
    for (const source of sources) {
      if (!source.path.includes(`${join("internal", "ui")}`)) {
        continue;
      }
      for (const match of source.text.matchAll(/provider: "([a-z0-9]+)"/g)) {
        uiProviders.add(match[1]);
      }
    }

    for (const provider of new Set([...providers, ...uiProviders])) {
      expect(providers).toContain(provider);
      for (const suffix of ["", "_fail", "_disable", "_auto"]) {
        expect(CLI_TELEMETRY_EVENTS.has(`sync_${provider}${suffix}`)).toBe(true);
      }
    }
  });

  test("allows boxd lifecycle verbs and onboarding choices", () => {
    const boxd = readFileSync(join(bastRoot, "internal/cli/boxd.go"), "utf8");
    const body = functionBody(boxd, "func (r *Runner) boxdLifecycle");
    const verbs = [...body.matchAll(/case "([a-z]+)"/g)].map((match) => match[1]);
    expect(verbs).toEqual(["stop", "start", "restart"]);
    for (const verb of verbs) {
      expect(CLI_TELEMETRY_EVENTS.has(`boxd_${verb}`)).toBe(true);
      expect(CLI_TELEMETRY_EVENTS.has(`boxd_${verb}_fail`)).toBe(true);
    }

    const onboardingFiles = ["internal/ui/onboarding.go", "internal/ui/input.go"];
    for (const relative of onboardingFiles) {
      const text = readFileSync(join(bastRoot, relative), "utf8");
      for (const match of text.matchAll(/"(onboarding_[a-z_]+)"/g)) {
        expect(CLI_TELEMETRY_EVENTS.has(match[1])).toBe(true);
      }
    }
  });

  test("allows vault events passed through trackVaultCall", () => {
    const vault = readFileSync(join(bastRoot, "internal/cli/vault.go"), "utf8");
    const calls = [...vault.matchAll(/trackVaultCall\(\s*"([^"]+)"\s*,\s*"([^"]+)"/g)];
    expect(calls.length).toBeGreaterThan(0);
    for (const [, okEvent, failEvent] of calls) {
      expect(CLI_TELEMETRY_EVENTS.has(okEvent)).toBe(true);
      expect(CLI_TELEMETRY_EVENTS.has(failEvent)).toBe(true);
    }
  });
});
