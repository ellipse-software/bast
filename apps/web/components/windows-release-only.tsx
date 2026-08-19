import type { ReactNode } from "react";

import { getLatestBastVersion } from "@/lib/github";
import { supportsWindowsRelease } from "@/lib/releases";

export async function WindowsReleaseOnly({
  children,
}: {
  children: ReactNode;
}) {
  const version = await getLatestBastVersion();
  return supportsWindowsRelease(version) ? children : null;
}
