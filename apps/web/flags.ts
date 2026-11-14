import { flag } from "flags/next";

import { runtimeVercelAdapter } from "@/lib/flags-client";

export const winget = flag<boolean>({
  key: "winget",
  adapter: runtimeVercelAdapter<boolean>(),
  defaultValue: false,
});
