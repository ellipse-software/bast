import { bastRepoUrl } from "@/lib/github";

export type InstallPlatform = "macos" | "linux" | "windows";
export type InstallMethod =
  | "script"
  | "powershell"
  | "winget"
  | "homebrew"
  | "apt"
  | "dnf"
  | "pacman"
  | "apk"
  | "source";

export type NavigatorPlatformHints = {
  userAgent: string;
  platform: string;
  userAgentDataPlatform?: string;
};

const PLATFORM_OPTIONS: { id: InstallPlatform; label: string }[] = [
  { id: "macos", label: "macOS" },
  { id: "linux", label: "Linux" },
  { id: "windows", label: "Windows" },
];

const METHOD_LABELS: Record<InstallMethod, string> = {
  script: "Script",
  powershell: "PowerShell",
  winget: "WinGet",
  homebrew: "Homebrew",
  apt: "apt",
  dnf: "dnf",
  pacman: "pacman",
  apk: "apk",
  source: "Source",
};

const MACOS_METHODS: InstallMethod[] = ["script", "homebrew", "source"];
const LINUX_METHODS: InstallMethod[] = [
  "script",
  "apt",
  "dnf",
  "pacman",
  "apk",
  "homebrew",
  "source",
];
const WINDOWS_METHODS: InstallMethod[] = ["powershell", "winget"];

export function installPlatforms(
  windowsAvailable: boolean,
): { id: InstallPlatform; label: string }[] {
  return windowsAvailable
    ? PLATFORM_OPTIONS
    : PLATFORM_OPTIONS.filter(({ id }) => id !== "windows");
}

export function methodsForPlatform(
  platform: InstallPlatform,
  wingetAvailable = false,
): { id: InstallMethod; label: string }[] {
  const ids: InstallMethod[] =
    platform === "windows"
      ? wingetAvailable
        ? WINDOWS_METHODS
        : ["powershell"]
      : platform === "linux"
        ? LINUX_METHODS
        : MACOS_METHODS;
  return ids.map((id) => ({ id, label: METHOD_LABELS[id] }));
}

export function defaultMethodFor(platform: InstallPlatform): InstallMethod {
  return platform === "windows" ? "powershell" : "script";
}

export function resolveMethod(
  platform: InstallPlatform,
  current: InstallMethod,
): InstallMethod {
  return methodsForPlatform(platform).some(({ id }) => id === current)
    ? current
    : defaultMethodFor(platform);
}

export function resolveDetectedPlatform(
  detected: InstallPlatform,
  windowsAvailable: boolean,
): InstallPlatform {
  if (detected === "windows" && !windowsAvailable) return "macos";
  return detected;
}

export function methodSupportsNightly(method: InstallMethod): boolean {
  return method === "script" || method === "powershell" || method === "homebrew";
}

export function promptFor(method: InstallMethod): string {
  return method === "powershell" || method === "winget" ? "PS>" : "$";
}

export function installCommand(
  method: InstallMethod,
  nightly: boolean,
): string {
  const useNightly = nightly && methodSupportsNightly(method);

  switch (method) {
    case "script":
      return useNightly
        ? "curl -fsSL https://bast.sh/install-nightly | sh"
        : "curl -fsSL https://bast.sh/install | sh";
    case "homebrew":
      return useNightly
        ? "brew install ellipse-software/tap/bast-nightly"
        : "brew install ellipse-software/tap/bast";
    case "apt":
      return "curl -fsSL https://packages.bast.sh/setup.sh | sudo sh -s -- apt";
    case "dnf":
      return "curl -fsSL https://packages.bast.sh/setup.sh | sudo sh -s -- dnf";
    case "pacman":
      return "curl -fsSL https://packages.bast.sh/setup.sh | sudo sh -s -- pacman";
    case "apk":
      return "curl -fsSL https://packages.bast.sh/setup.sh | sudo sh -s -- apk";
    case "powershell":
      return useNightly
        ? "irm https://bast.sh/install-nightly.ps1 | iex"
        : "irm https://bast.sh/install.ps1 | iex";
    case "winget":
      return "winget install EllipseSoftware.Bast";
    case "source":
      return `git clone ${bastRepoUrl}.git && cd bast/apps/bast && go build -trimpath -o bast .`;
  }
}

export function detectInstallPlatform(
  hints: NavigatorPlatformHints,
): InstallPlatform {
  const hint = hints.userAgentDataPlatform?.trim().toLowerCase() ?? "";
  if (hint === "windows" || hint.includes("win")) return "windows";
  if (hint === "macos" || hint === "ios" || hint.includes("mac")) {
    return "macos";
  }
  if (hint === "linux" || hint === "android" || hint.includes("linux")) {
    return "linux";
  }

  const platform = hints.platform.toLowerCase();
  const ua = hints.userAgent;

  if (platform.startsWith("win") || /windows/i.test(ua)) return "windows";
  if (
    platform.includes("mac") ||
    /Mac OS|Macintosh|iPhone|iPad|iPod/.test(ua)
  ) {
    return "macos";
  }
  if (platform.includes("linux") || /Linux|Android/i.test(ua)) return "linux";

  return "macos";
}

export function detectInstallPlatformFromNavigator(nav: {
  userAgent: string;
  platform: string;
  userAgentData?: { platform?: string };
}): InstallPlatform {
  return detectInstallPlatform({
    userAgent: nav.userAgent,
    platform: nav.platform,
    userAgentDataPlatform: nav.userAgentData?.platform,
  });
}

export function subscribeInstallPlatform() {
  return () => {};
}

export function getClientInstallPlatform(): InstallPlatform {
  if (typeof navigator === "undefined") return "macos";
  return detectInstallPlatformFromNavigator(navigator);
}

export function getServerInstallPlatform(): InstallPlatform {
  return "macos";
}
