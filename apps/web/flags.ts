import { vercelAdapter } from "@flags-sdk/vercel";
import { flag } from "flags/next";

export const winget = flag<boolean>({
  key: "winget",
  adapter: vercelAdapter(),
  defaultValue: false,
});
