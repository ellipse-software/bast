import {
  aggregateLabel,
  serviceStatusLabel,
  type AggregateStatus,
  type ServiceStatus,
  type StatusDay,
  type StatusIncident,
  type StatusPageData,
  type StatusService,
} from "@/lib/betterstack";

function formatAvailability(value: number | null): string {
  if (value === null || Number.isNaN(value)) return "—";
  if (value >= 99.995) return "100%";
  return `${value.toFixed(2)}%`;
}

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("en-GB", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(date);
}

function formatDayTitle(day: StatusDay): string {
  if (day.status === "empty") {
    return `${day.day}: No data`;
  }
  if (day.status === "downtime") {
    const minutes = Math.max(1, Math.round(day.downtimeSeconds / 60));
    return `${day.day}: ${minutes}m downtime`;
  }
  return `${day.day}: Operational`;
}

function aggregateTone(status: AggregateStatus): string {
  switch (status) {
    case "operational":
      return "border-[#3fb950]/40 bg-[#1a7f37]/15 text-[#3fb950]";
    case "downtime":
      return "border-[#f85149]/40 bg-[#cf222e]/15 text-[#f85149]";
    case "maintenance":
      return "border-[#d4a72c]/40 bg-[#9a6700]/20 text-[#d4a72c]";
    case "degraded":
      return "border-[#d4a72c]/40 bg-[#9a6700]/20 text-[#d4a72c]";
    case "unknown":
      return "border-border bg-surface text-muted";
  }
}

function serviceTone(status: ServiceStatus): string {
  switch (status) {
    case "operational":
      return "text-[#3fb950]";
    case "downtime":
      return "text-[#f85149]";
    case "maintenance":
      return "text-[#d4a72c]";
    case "unknown":
      return "text-muted";
  }
}

function dayBarClass(status: StatusDay["status"]): string {
  switch (status) {
    case "operational":
      return "bg-[#3fb950]";
    case "downtime":
      return "bg-[#f85149]";
    case "empty":
      return "bg-border";
  }
}

function HistoryBar({ history }: { history: StatusDay[] }) {
  return (
    <div
      className="flex h-8 w-full items-stretch gap-px"
      role="img"
      aria-label="90-day uptime history"
    >
      {history.map((day) => (
        <div
          key={day.day}
          title={formatDayTitle(day)}
          className={`min-w-0 flex-1 ${dayBarClass(day.status)}`}
        />
      ))}
    </div>
  );
}

function ServiceRow({ service }: { service: StatusService }) {
  return (
    <div className="px-4 py-5 sm:px-5">
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <div className="min-w-0">
          <p className="text-sm font-medium tracking-tight text-foreground">
            {service.name}
          </p>
          {!service.configured || service.error ? (
            <p className="mt-0.5 text-xs text-muted">
              {!service.configured
                ? "Monitor not configured"
                : service.error}
            </p>
          ) : null}
        </div>
        <div className="flex items-baseline gap-3 text-sm">
          <span className="tabular-nums text-muted">
            {formatAvailability(service.availability)}
            <span className="ml-1 text-xs">uptime</span>
          </span>
          <span className={`font-medium ${serviceTone(service.status)}`}>
            {serviceStatusLabel(service.status)}
          </span>
        </div>
      </div>
      <HistoryBar history={service.history} />
      <div className="mt-2 flex justify-between text-xs text-muted">
        <span>90 days ago</span>
        <span>Today</span>
      </div>
    </div>
  );
}

function IncidentsList({ incidents }: { incidents: StatusIncident[] }) {
  if (incidents.length === 0) {
    return <p className="text-sm text-muted">No incidents reported.</p>;
  }

  return (
    <ul className="divide-y divide-border border border-border">
      {incidents.map((incident) => (
        <li key={incident.id} className="px-4 py-4 sm:px-5">
          <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
            <p className="text-sm font-medium tracking-tight text-foreground">
              {incident.name}
            </p>
            <p className="text-xs text-muted">{incident.serviceName}</p>
          </div>
          {incident.cause ? (
            <p className="mt-1 text-sm text-muted">{incident.cause}</p>
          ) : null}
          <p className="mt-2 text-xs text-muted">
            <span>{incident.status}</span>
            <span className="mx-1.5 text-border">·</span>
            <time dateTime={incident.startedAt}>
              {formatDateTime(incident.startedAt)}
            </time>
            {incident.resolvedAt ? (
              <>
                <span className="mx-1.5 text-border">→</span>
                <time dateTime={incident.resolvedAt}>
                  {formatDateTime(incident.resolvedAt)}
                </time>
              </>
            ) : null}
          </p>
        </li>
      ))}
    </ul>
  );
}

export function StatusPageView({ data }: { data: StatusPageData }) {
  return (
    <div className="w-full">
      <div
        className={`mb-10 border px-5 py-6 sm:px-6 sm:py-7 ${aggregateTone(data.aggregate)}`}
      >
        <p className="text-xl font-medium tracking-tight sm:text-2xl">
          {aggregateLabel(data.aggregate)}
        </p>
        <p className="mt-2 text-sm opacity-80">
          {data.configured
            ? "Uptime over the past 90 days."
            : "Connect Better Stack monitors to show live uptime."}
        </p>
      </div>

      <section className="mb-12">
        <h2 className="mb-4 text-sm font-medium tracking-tight text-foreground">
          Services
        </h2>
        <div className="divide-y divide-border border border-border bg-background">
          {data.services.map((service) => (
            <ServiceRow key={service.key} service={service} />
          ))}
        </div>
        <div className="mt-4 flex flex-wrap gap-x-4 gap-y-2 text-xs text-muted">
          <span className="inline-flex items-center gap-1.5">
            <span
              className="inline-block size-2.5 bg-[#3fb950]"
              aria-hidden
            />
            Operational
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span
              className="inline-block size-2.5 bg-[#f85149]"
              aria-hidden
            />
            Downtime
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="inline-block size-2.5 bg-border" aria-hidden />
            No data
          </span>
        </div>
      </section>

      <section>
        <h2 className="mb-4 text-sm font-medium tracking-tight text-foreground">
          Past incidents
        </h2>
        <IncidentsList incidents={data.incidents} />
      </section>
    </div>
  );
}
