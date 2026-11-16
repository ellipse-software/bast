import Link from "next/link";

type Row = {
  question: string;
  bast: string;
  termius: string;
  putty: string;
};

const rows: Row[] = [
  {
    question: "Where does it run?",
    bast: "Inside your terminal on macOS, Linux, and Windows 11",
    termius: "As a separate GUI app on desktop and phones",
    putty: "As a Windows desktop app",
  },
  {
    question: "How do you connect?",
    bast: "Calls the OpenSSH binary already on your machine",
    termius: "Uses Termius's own SSH client",
    putty: "Uses PuTTY's own SSH stack and .ppk keys",
  },
  {
    question: "Where are hosts stored?",
    bast: "Your normal ~/.ssh/config (Bast does not replace it)",
    termius: "Inside Termius's own host database",
    putty: "As saved PuTTY sessions",
  },
  {
    question: "Cloud hosts?",
    bast: "CLIs for GCP, AWS, Azure, Boat, Fly.io, DigitalOcean, and boxd; APIs for Hetzner, Upstash, Vercel, BitQate, and Railway",
    termius: "Mostly manual setup",
    putty: "Not built in",
  },
  {
    question: "Sync between machines?",
    bast: "Optional encrypted Bast vault for managed hosts and keys",
    termius: "Through a Termius account",
    putty: "No",
  },
  {
    question: "File transfers?",
    bast: "Dual-pane SFTP in the same TUI",
    termius: "Built-in SFTP UI on paid plans",
    putty: "Separate PSCP / PSFTP tools",
  },
  {
    question: "Cost?",
    bast: "Free and open source (MIT)",
    termius: "Free tier, paid plans for sync and extras",
    putty: "Free",
  },
];

const bastCell = "bg-[color-mix(in_srgb,#c6b6f2_8%,#05080f)] text-[#e8eef6]";
const nightCell = "border-b border-white/15 px-4 py-3.5 align-top leading-relaxed";

function NightComparison() {
  return (
    <section className="w-full">
      <div className="mb-6 flex flex-wrap items-baseline justify-between gap-x-8 gap-y-2">
        <h2 className="text-[1.65rem] font-medium leading-none tracking-[-0.03em]">
          Bast, Termius, or PuTTY?
        </h2>
        <Link
          href="/alternatives"
          className="text-sm text-[#9aa8bc] outline-none transition-colors hover:text-[#e8eef6] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-[#c6b6f2]"
        >
          Other comparisons
        </Link>
      </div>
      <div className="relative">
        <div
          tabIndex={0}
          className="overflow-x-auto outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-[#c6b6f2]"
        >
          <table className="w-full min-w-[46rem] border-collapse text-left">
            <thead>
              <tr>
                <th
                  scope="col"
                  className="sticky top-0 left-0 z-20 w-[8.5rem] min-w-[8.5rem] border-b border-white/15 bg-[#05080f] px-4 py-3 text-[13px] font-medium text-[#9aa8bc]"
                >
                  <span className="sr-only">Question</span>
                </th>
                <th
                  scope="col"
                  className={`sticky top-0 z-10 min-w-[12.5rem] border-b border-white/15 px-4 py-3 font-medium ${bastCell}`}
                >
                  Bast
                  <span className="mt-0.5 block text-xs font-normal text-[#9aa8bc]">
                    Terminal picker
                  </span>
                </th>
                <th
                  scope="col"
                  className="sticky top-0 z-10 min-w-[12.5rem] border-b border-white/15 bg-[#05080f] px-4 py-3 font-medium text-[#e8eef6]"
                >
                  Termius
                  <span className="mt-0.5 block text-xs font-normal text-[#9aa8bc]">
                    GUI SSH client
                  </span>
                </th>
                <th
                  scope="col"
                  className="sticky top-0 z-10 min-w-[12.5rem] border-b border-white/15 bg-[#05080f] px-4 py-3 font-medium text-[#e8eef6]"
                >
                  PuTTY
                  <span className="mt-0.5 block text-xs font-normal text-[#9aa8bc]">
                    Classic Windows client
                  </span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.question}>
                  <th
                    scope="row"
                    className={`${nightCell} sticky left-0 z-10 w-[8.5rem] min-w-[8.5rem] bg-[#05080f] text-[13px] font-medium text-[#9aa8bc]`}
                  >
                    {row.question}
                  </th>
                  <td className={`${nightCell} text-sm ${bastCell}`}>{row.bast}</td>
                  <td className={`${nightCell} text-sm text-[#c9d5e6]`}>{row.termius}</td>
                  <td className={`${nightCell} text-sm text-[#c9d5e6]`}>{row.putty}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div
          aria-hidden
          className="pointer-events-none absolute inset-y-0 right-0 w-10 bg-gradient-to-l from-[#05080f] to-transparent lg:hidden"
        />
      </div>
    </section>
  );
}

export function Comparison({ tone = "default" }: { tone?: "default" | "night" }) {
  if (tone === "night") return <NightComparison />;

  return (
    <section className="w-full">
      <div className="mb-6 max-w-xl">
        <h2 className="mb-2 text-lg font-medium tracking-tight">
          Bast, Termius, or PuTTY?
        </h2>
        <p className="text-sm leading-relaxed text-muted">
          All three get you onto servers. Bast is for people who already live in
          the terminal and want a faster way around OpenSSH, not another app
          that owns your hosts. See all{" "}
          <Link
            href="/alternatives"
            className="text-foreground underline-offset-2 hover:underline"
          >
            comparisons
          </Link>
          : Termius, PuTTY, MobaXterm, SecureCRT.
        </p>
      </div>

      <div className="overflow-x-auto bg-border p-px">
        <table className="w-full min-w-[36rem] border-collapse text-left text-sm">
          <thead>
            <tr>
              <th
                scope="col"
                className="w-[9.5rem] border-b border-border bg-background px-4 py-3 font-medium text-muted"
              >
                <span className="sr-only">Question</span>
              </th>
              <th
                scope="col"
                className="border-b border-border bg-[color-mix(in_srgb,var(--accent)_9%,var(--background))] px-4 py-3 font-medium text-foreground"
              >
                Bast
                <span className="mt-0.5 block text-xs font-normal text-muted">
                  Terminal picker
                </span>
              </th>
              <th
                scope="col"
                className="border-b border-border bg-background px-4 py-3 font-medium text-muted"
              >
                Termius
                <span className="mt-0.5 block text-xs font-normal">
                  GUI SSH client
                </span>
              </th>
              <th
                scope="col"
                className="border-b border-border bg-background px-4 py-3 font-medium text-muted"
              >
                PuTTY
                <span className="mt-0.5 block text-xs font-normal">
                  Classic Windows client
                </span>
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, index) => {
              const isLast = index === rows.length - 1;
              const edge = isLast ? "" : "border-b border-border";
              return (
                <tr key={row.question}>
                  <th
                    scope="row"
                    className={`${edge} bg-background px-4 py-3.5 align-top font-medium text-foreground`}
                  >
                    {row.question}
                  </th>
                  <td
                    className={`${edge} bg-[color-mix(in_srgb,var(--accent)_9%,var(--background))] px-4 py-3.5 align-top leading-relaxed text-foreground`}
                  >
                    {row.bast}
                  </td>
                  <td
                    className={`${edge} bg-background px-4 py-3.5 align-top leading-relaxed text-muted`}
                  >
                    {row.termius}
                  </td>
                  <td
                    className={`${edge} bg-background px-4 py-3.5 align-top leading-relaxed text-muted`}
                  >
                    {row.putty}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}
