import { redirect } from "next/navigation";

import { SPONSOR_OPEN_HREF } from "@/lib/checkout-return";

export default function SponsorPage() {
  redirect(SPONSOR_OPEN_HREF);
}
