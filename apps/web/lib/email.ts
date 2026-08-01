import { createEmailClient } from "@opencoredev/email-sdk";
import { cloudflare } from "@opencoredev/email-sdk/cloudflare";

let client: ReturnType<typeof createEmailClient> | null = null;

function emailConfigured(): boolean {
  return Boolean(
    process.env.CLOUDFLARE_API_TOKEN &&
      (process.env.CLOUDFLARE_ACCOUNT_ID || process.env.R2_ACCOUNT_ID),
  );
}

function getEmailClient() {
  if (client) return client;
  const apiToken = process.env.CLOUDFLARE_API_TOKEN;
  const accountId = process.env.CLOUDFLARE_ACCOUNT_ID || process.env.R2_ACCOUNT_ID;
  if (!apiToken || !accountId) {
    throw new Error("CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID (or R2_ACCOUNT_ID) are required");
  }
  client = createEmailClient({
    adapters: [cloudflare({ apiToken, accountId })],
  });
  return client;
}

export async function sendVaultOTP(to: string, otp: string): Promise<void> {
  const from = process.env.CLOUDFLARE_EMAIL_FROM || "Bast <vault@bast.sh>";
  if (!emailConfigured()) {
    if (process.env.NODE_ENV !== "production") {
      console.info(`[vault-otp] ${to}: ${otp}`);
      return;
    }
    throw new Error("CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID are required to send vault OTP email");
  }
  await getEmailClient().send({
    from,
    to,
    subject: `${otp} is your Bast vault code`,
    text: `Hi there,\n\nYour Bast vault sign-in code is ${otp}. It expires in 10 minutes.\n\nIf you did not request this, you can ignore this email.\n\nThanks,\nTed at Bast.sh`,
  });
}
