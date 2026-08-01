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
    bast: "Inside your terminal on macOS and Linux",
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
    question: "Cloud VMs?",
    bast: "Imports from GCP, AWS, and Azure via their CLIs",
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

export function Comparison() {
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
                className="border-b border-border bg-surface px-4 py-3 font-medium text-foreground"
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
            {rows.map((row) => (
              <tr key={row.question}>
                <th
                  scope="row"
                  className="border-b border-border bg-background px-4 py-3.5 align-top font-medium text-foreground"
                >
                  {row.question}
                </th>
                <td className="border-b border-border bg-surface px-4 py-3.5 align-top leading-relaxed text-foreground">
                  {row.bast}
                </td>
                <td className="border-b border-border bg-background px-4 py-3.5 align-top leading-relaxed text-muted">
                  {row.termius}
                </td>
                <td className="border-b border-border bg-background px-4 py-3.5 align-top leading-relaxed text-muted">
                  {row.putty}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
