import { describe, expect, test } from "bun:test";

import {
  defaultMethodFor,
  detectInstallPlatform,
  installCommand,
  installPlatforms,
  methodSupportsNightly,
  methodsForPlatform,
  promptFor,
  resolveDetectedPlatform,
  resolveMethod,
} from "@/lib/install";

describe("installPlatforms", () => {
  test("omits Windows until a supporting release exists", () => {
    expect(installPlatforms(false).map(({ id }) => id)).toEqual([
      "macos",
      "linux",
    ]);
  });

  test("includes Windows once a supporting release exists", () => {
    expect(installPlatforms(true).map(({ id }) => id)).toEqual([
      "macos",
      "linux",
      "windows",
    ]);
  });
});

describe("methodsForPlatform", () => {
  test("unix platforms offer the script, Homebrew, and source", () => {
    expect(methodsForPlatform("macos").map(({ id }) => id)).toEqual([
      "script",
      "homebrew",
      "source",
    ]);
    expect(methodsForPlatform("linux").map(({ id }) => id)).toEqual([
      "script",
      "homebrew",
      "source",
    ]);
  });

  test("Windows offers PowerShell and WinGet", () => {
    expect(methodsForPlatform("windows").map(({ id }) => id)).toEqual([
      "powershell",
      "winget",
    ]);
  });
});

describe("resolveMethod", () => {
  test("keeps a method that is valid on the next platform", () => {
    expect(resolveMethod("linux", "homebrew")).toBe("homebrew");
  });

  test("falls back to the platform default when the method is unavailable", () => {
    expect(resolveMethod("windows", "script")).toBe("powershell");
    expect(resolveMethod("macos", "winget")).toBe("script");
    expect(defaultMethodFor("linux")).toBe("script");
  });
});

describe("resolveDetectedPlatform", () => {
  test("stays on the detected unix platform", () => {
    expect(resolveDetectedPlatform("linux", false)).toBe("linux");
    expect(resolveDetectedPlatform("macos", true)).toBe("macos");
  });

  test("falls back from Windows when that release is not available", () => {
    expect(resolveDetectedPlatform("windows", false)).toBe("macos");
    expect(resolveDetectedPlatform("windows", true)).toBe("windows");
  });
});

describe("detectInstallPlatform", () => {
  test("prefers userAgentData.platform", () => {
    expect(
      detectInstallPlatform({
        userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
        platform: "MacIntel",
        userAgentDataPlatform: "Windows",
      }),
    ).toBe("windows");
  });

  test("detects Windows from the user agent", () => {
    expect(
      detectInstallPlatform({
        userAgent:
          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
        platform: "Win32",
      }),
    ).toBe("windows");
  });

  test("detects macOS and iOS", () => {
    expect(
      detectInstallPlatform({
        userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
        platform: "MacIntel",
      }),
    ).toBe("macos");
    expect(
      detectInstallPlatform({
        userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
        platform: "iPhone",
      }),
    ).toBe("macos");
  });

  test("detects Linux", () => {
    expect(
      detectInstallPlatform({
        userAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
        platform: "Linux x86_64",
      }),
    ).toBe("linux");
  });

  test("defaults unknown platforms to macOS", () => {
    expect(
      detectInstallPlatform({
        userAgent: "Unknown",
        platform: "",
      }),
    ).toBe("macos");
  });
});

describe("installCommand", () => {
  test("returns the stable and nightly script commands", () => {
    expect(installCommand("script", false)).toBe(
      "curl -fsSL https://bast.sh/install | sh",
    );
    expect(installCommand("script", true)).toBe(
      "curl -fsSL https://bast.sh/install-nightly | sh",
    );
  });

  test("returns PowerShell and WinGet commands", () => {
    expect(installCommand("powershell", false)).toBe(
      "irm https://bast.sh/install.ps1 | iex",
    );
    expect(installCommand("powershell", true)).toBe(
      "irm https://bast.sh/install-nightly.ps1 | iex",
    );
    expect(installCommand("winget", false)).toBe(
      "winget install EllipseSoftware.Bast",
    );
    expect(installCommand("winget", true)).toBe(
      "winget install EllipseSoftware.Bast",
    );
  });

  test("returns Homebrew and source commands", () => {
    expect(installCommand("homebrew", false)).toBe(
      "brew install ellipse-software/tap/bast",
    );
    expect(installCommand("homebrew", true)).toBe(
      "brew install ellipse-software/tap/bast-nightly",
    );
    expect(installCommand("source", true)).toContain("go build");
  });

  test("only script, PowerShell, and Homebrew have a nightly channel", () => {
    expect(methodSupportsNightly("script")).toBe(true);
    expect(methodSupportsNightly("powershell")).toBe(true);
    expect(methodSupportsNightly("homebrew")).toBe(true);
    expect(methodSupportsNightly("winget")).toBe(false);
    expect(methodSupportsNightly("source")).toBe(false);
  });

  test("uses a PowerShell prompt for Windows methods", () => {
    expect(promptFor("powershell")).toBe("PS>");
    expect(promptFor("winget")).toBe("PS>");
    expect(promptFor("script")).toBe("$");
  });
});
