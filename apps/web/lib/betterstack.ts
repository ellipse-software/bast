const UPTIME_API = "https://uptime.betterstack.com";
const HISTORY_DAYS = 90;
const REVALIDATE_SECONDS = 60;

export type ServiceKey = "marketing" | "docs" | "vault";

export type ServiceStatus =
  | "operational"
  | "downtime"
  | "maintenance"
  | "unknown";

export type AggregateStatus =
  | "operational"
  | "downtime"
  | "maintenance"
  | "degraded"
  | "unknown";

export type DayHistoryStatus = "operational" | "downtime" | "empty";

export type StatusDay = {
  day: string;
  status: DayHistoryStatus;
  downtimeSeconds: number;
};

export type StatusIncident = {
  id: string;
  name: string;
  cause: string | null;
  serviceKey: ServiceKey;
  serviceName: string;
  startedAt: string;
  resolvedAt: string | null;
  status: string;
};

export type StatusService = {
  key: ServiceKey;
  name: string;
  monitorId: string | null;
  status: ServiceStatus;
  availability: number | null;
  lastCheckedAt: string | null;
  createdAt: string | null;
  history: StatusDay[];
  configured: boolean;
  error: string | null;
};

export type StatusPageData = {
  configured: boolean;
  aggregate: AggregateStatus;
  services: StatusService[];
  incidents: StatusIncident[];
  fetchedAt: string;
};

const SERVICE_DEFS: ReadonlyArray<{
  key: ServiceKey;
  name: string;
  envKey:
    | "BETTERSTACK_MONITOR_MARKETING"
    | "BETTERSTACK_MONITOR_DOCS"
    | "BETTERSTACK_MONITOR_VAULT";
}> = [
  {
    key: "marketing",
    name: "Marketing site",
    envKey: "BETTERSTACK_MONITOR_MARKETING",
  },
  { key: "docs", name: "Docs", envKey: "BETTERSTACK_MONITOR_DOCS" },
  { key: "vault", name: "Vault", envKey: "BETTERSTACK_MONITOR_VAULT" },
];

type MonitorAttributes = {
  pronounceable_name?: string;
  status?: string;
  last_checked_at?: string | null;
  created_at?: string | null;
};

type MonitorResponse = {
  data?: {
    id?: string;
    attributes?: MonitorAttributes;
  };
};

type SlaResponse = {
  data?: {
    attributes?: {
      availability?: number;
      total_downtime?: number;
    };
  };
};

type IncidentAttributes = {
  name?: string;
  cause?: string | null;
  started_at?: string;
  resolved_at?: string | null;
  status?: string;
};

type IncidentResource = {
  id: string;
  attributes?: IncidentAttributes;
  relationships?: {
    monitor?: {
      data?: { id?: string; type?: string } | null;
    };
  };
};

type IncidentsResponse = {
  data?: IncidentResource[];
  pagination?: {
    next?: string | null;
  };
};

function getToken(): string | null {
  const token = process.env.BETTERSTACK_API_TOKEN?.trim();
  return token || null;
}

function getMonitorId(envKey: (typeof SERVICE_DEFS)[number]["envKey"]): string | null {
  const id = process.env[envKey]?.trim();
  return id || null;
}

function isoDate(date: Date): string {
  return date.toISOString().slice(0, 10);
}

function addDays(date: Date, days: number): Date {
  const next = new Date(date);
  next.setUTCDate(next.getUTCDate() + days);
  return next;
}

function startOfUtcDay(iso: string): Date {
  return new Date(`${iso.slice(0, 10)}T00:00:00.000Z`);
}

function mapMonitorStatus(raw: string | undefined): ServiceStatus {
  switch (raw) {
    case "up":
    case "validating":
      return "operational";
    case "down":
      return "downtime";
    case "maintenance":
      return "maintenance";
    default:
      return "unknown";
  }
}

function deriveAggregate(services: StatusService[]): AggregateStatus {
  if (services.every((service) => !service.configured)) {
    return "unknown";
  }

  const active = services.filter((service) => service.configured);
  if (active.some((service) => service.status === "downtime")) {
    return "downtime";
  }
  if (active.some((service) => service.status === "maintenance")) {
    return "maintenance";
  }
  if (active.some((service) => service.status === "unknown" || service.error)) {
    return "degraded";
  }
  if (active.every((service) => service.status === "operational")) {
    return "operational";
  }
  return "degraded";
}

async function betterstackFetch<T>(
  path: string,
  token: string,
): Promise<{ ok: true; data: T } | { ok: false; error: string }> {
  try {
    const response = await fetch(`${UPTIME_API}${path}`, {
      headers: {
        Authorization: `Bearer ${token}`,
        Accept: "application/json",
        "User-Agent": "bast.sh-status",
      },
      next: { revalidate: REVALIDATE_SECONDS },
    });

    if (!response.ok) {
      return {
        ok: false,
        error: `Better Stack responded ${response.status}`,
      };
    }

    return { ok: true, data: (await response.json()) as T };
  } catch {
    return { ok: false, error: "Failed to reach Better Stack" };
  }
}

function emptyHistory(createdAt: string | null): StatusDay[] {
  const today = startOfUtcDay(isoDate(new Date()));
  const createdDay = createdAt ? startOfUtcDay(createdAt) : null;
  const days: StatusDay[] = [];

  for (let i = HISTORY_DAYS - 1; i >= 0; i -= 1) {
    const day = addDays(today, -i);
    const dayKey = isoDate(day);
    const beforeCreation = createdDay !== null && day < createdDay;
    days.push({
      day: dayKey,
      status: beforeCreation ? "empty" : "operational",
      downtimeSeconds: 0,
    });
  }

  return days;
}

function buildHistoryFromIncidents(
  createdAt: string | null,
  incidents: Array<{ startedAt: string; resolvedAt: string | null }>,
): StatusDay[] {
  const days = emptyHistory(createdAt);
  const byDay = new Map(days.map((day) => [day.day, day]));
  const now = Date.now();

  for (const incident of incidents) {
    const start = new Date(incident.startedAt).getTime();
    if (Number.isNaN(start)) continue;
    const end = incident.resolvedAt
      ? new Date(incident.resolvedAt).getTime()
      : now;
    if (Number.isNaN(end) || end < start) continue;

    for (const day of days) {
      if (day.status === "empty") continue;
      const dayStart = startOfUtcDay(day.day).getTime();
      const dayEnd = dayStart + 24 * 60 * 60 * 1000;
      const overlapStart = Math.max(start, dayStart);
      const overlapEnd = Math.min(end, dayEnd);
      if (overlapEnd <= overlapStart) continue;

      const existing = byDay.get(day.day);
      if (!existing || existing.status === "empty") continue;
      existing.status = "downtime";
      existing.downtimeSeconds += Math.round((overlapEnd - overlapStart) / 1000);
    }
  }

  return days;
}

async function fetchMonitorIncidents(
  token: string,
  monitorId: string,
  from: string,
  to: string,
): Promise<IncidentResource[]> {
  const path = `/api/v3/incidents?monitor_id=${encodeURIComponent(monitorId)}&from=${from}&to=${to}&per_page=50`;
  const result:
    | { ok: true; data: IncidentsResponse }
    | { ok: false; error: string } = await betterstackFetch<IncidentsResponse>(
    path,
    token,
  );
  if (!result.ok) return [];
  return result.data.data ?? [];
}

function unknownService(
  key: ServiceKey,
  name: string,
  monitorId: string | null,
  error: string | null,
): StatusService {
  return {
    key,
    name,
    monitorId,
    status: "unknown",
    availability: null,
    lastCheckedAt: null,
    createdAt: null,
    history: emptyHistory(null).map((day) => ({ ...day, status: "empty" })),
    configured: Boolean(monitorId),
    error,
  };
}

async function loadService(
  token: string,
  key: ServiceKey,
  name: string,
  monitorId: string,
  from: string,
  to: string,
): Promise<{ service: StatusService; incidents: StatusIncident[] }> {
  const [monitorResult, slaResult, incidentResources] = await Promise.all([
    betterstackFetch<MonitorResponse>(`/api/v2/monitors/${monitorId}`, token),
    betterstackFetch<SlaResponse>(
      `/api/v2/monitors/${monitorId}/sla?from=${from}&to=${to}`,
      token,
    ),
    fetchMonitorIncidents(token, monitorId, from, to),
  ]);

  if (!monitorResult.ok) {
    return {
      service: unknownService(key, name, monitorId, monitorResult.error),
      incidents: [],
    };
  }

  const attributes = monitorResult.data.data?.attributes ?? {};
  const displayName = attributes.pronounceable_name?.trim() || name;
  const createdAt = attributes.created_at ?? null;
  const mappedIncidents = incidentResources
    .map((incident): StatusIncident | null => {
      const startedAt = incident.attributes?.started_at;
      if (!startedAt) return null;
      return {
        id: incident.id,
        name: incident.attributes?.name?.trim() || displayName,
        cause: incident.attributes?.cause?.trim() || null,
        serviceKey: key,
        serviceName: displayName,
        startedAt,
        resolvedAt: incident.attributes?.resolved_at ?? null,
        status: incident.attributes?.status?.trim() || "Started",
      };
    })
    .filter((incident): incident is StatusIncident => incident !== null);

  const history = buildHistoryFromIncidents(
    createdAt,
    mappedIncidents.map((incident) => ({
      startedAt: incident.startedAt,
      resolvedAt: incident.resolvedAt,
    })),
  );

  return {
    service: {
      key,
      name: displayName,
      monitorId,
      status: mapMonitorStatus(attributes.status),
      availability: slaResult.ok
        ? (slaResult.data.data?.attributes?.availability ?? null)
        : null,
      lastCheckedAt: attributes.last_checked_at ?? null,
      createdAt,
      history,
      configured: true,
      error: slaResult.ok ? null : slaResult.error,
    },
    incidents: mappedIncidents,
  };
}

export async function getStatusPageData(): Promise<StatusPageData> {
  const fetchedAt = new Date().toISOString();
  const token = getToken();
  const defs = SERVICE_DEFS.map((def) => ({
    ...def,
    monitorId: getMonitorId(def.envKey),
  }));

  if (!token || defs.every((def) => !def.monitorId)) {
    return {
      configured: false,
      aggregate: "unknown",
      services: defs.map((def) =>
        unknownService(
          def.key,
          def.name,
          def.monitorId,
          token ? "Monitor id not set" : "BETTERSTACK_API_TOKEN is not set",
        ),
      ),
      incidents: [],
      fetchedAt,
    };
  }

  const to = isoDate(new Date());
  const from = isoDate(addDays(startOfUtcDay(to), -(HISTORY_DAYS - 1)));

  const results = await Promise.all(
    defs.map(async (def) => {
      if (!def.monitorId) {
        return {
          service: unknownService(
            def.key,
            def.name,
            null,
            "Monitor id not set",
          ),
          incidents: [] as StatusIncident[],
        };
      }
      return loadService(token, def.key, def.name, def.monitorId, from, to);
    }),
  );

  const services = results.map((result) => result.service);
  const incidents = results
    .flatMap((result) => result.incidents)
    .sort(
      (a, b) =>
        new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime(),
    );

  return {
    configured: defs.some((def) => Boolean(def.monitorId)),
    aggregate: deriveAggregate(services),
    services,
    incidents,
    fetchedAt,
  };
}

export function aggregateLabel(status: AggregateStatus): string {
  switch (status) {
    case "operational":
      return "All systems operational";
    case "downtime":
      return "Major outage";
    case "maintenance":
      return "Under maintenance";
    case "degraded":
      return "Partial outage";
    case "unknown":
      return "Status unavailable";
  }
}

export function serviceStatusLabel(status: ServiceStatus): string {
  switch (status) {
    case "operational":
      return "Operational";
    case "downtime":
      return "Downtime";
    case "maintenance":
      return "Maintenance";
    case "unknown":
      return "Unknown";
  }
}
