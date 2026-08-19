const stableVersionPattern = /^v?(\d+)\.(\d+)\.(\d+)$/;
const windowsReleaseBlockPattern =
  /<WindowsReleaseOnly>\s*([\s\S]*?)\s*<\/WindowsReleaseOnly>/g;

export const preWindowsInstallDescription =
  "Install Bast on macOS or Linux via script, Homebrew, or from source.";

export function supportsWindowsRelease(version?: string | null): boolean {
  if (!version) return false;
  const match = stableVersionPattern.exec(version);
  if (!match) return false;

  const major = Number(match[1]);
  const minor = Number(match[2]);
  return major > 0 || minor >= 9;
}

export function resolveWindowsReleaseContent(
  content: string,
  windowsAvailable: boolean,
): string {
  return content.replace(
    windowsReleaseBlockPattern,
    windowsAvailable ? "$1" : "",
  );
}
