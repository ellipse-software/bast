"use client";

import { useCallback, useMemo, useRef, useState } from "react";

type Section = "hosts" | "keys" | "sync";
type SyncProvider = "" | "gcp" | "aws" | "azure";

type DetailRow = { label: string; value: string };

type HostItem = {
  name: string;
  alias?: string;
  favorite?: boolean;
  synced?: "gcp" | "aws" | "azure";
  destination: string;
  summary: string;
  rows: DetailRow[];
};

type KeyItem = {
  name: string;
  inAgent?: boolean;
  summary: string;
  fingerprint?: string;
  rows: DetailRow[];
};

type HostListRow =
  | {
      kind: "group";
      id: string;
      label: string;
      depth: number;
      count: number;
      googleCloud?: boolean;
      synced?: boolean;
    }
  | { kind: "host"; id: string; host: HostItem; depth: number };

type SyncMenuItem = {
  label: string;
  detail?: string;
  disabled?: boolean;
  action?: "open-gcp" | "open-aws" | "open-azure";
};

type HostGroupNode = {
  path: string;
  label: string;
  googleCloud?: boolean;
  synced?: boolean;
  hosts?: HostItem[];
  children?: HostGroupNode[];
};

const hostTree: HostGroupNode[] = [
  {
    path: "production",
    label: "production",
    hosts: [
      {
        name: "api",
        favorite: true,
        destination: "deploy@api.prod.example.com",
        summary: "Bast managed · known host",
        rows: [
          { label: "Source", value: "~/.ssh/bast/config:12" },
          { label: "Key", value: "~/.ssh/bast/keys/work" },
          { label: "Meta", value: "prod · web, api" },
          { label: "Used", value: "2026-07-26 14:02 (18)" },
        ],
      },
      {
        name: "bastion",
        destination: "ops@bastion.prod.example.com",
        summary: "Bast managed · known host",
        rows: [
          { label: "Source", value: "~/.ssh/bast/config:28" },
          { label: "Key", value: "~/.ssh/bast/keys/work" },
          { label: "Meta", value: "prod · jump" },
        ],
      },
    ],
  },
  {
    path: "Google Cloud",
    label: "Google Cloud",
    googleCloud: true,
    children: [
      {
        path: "Google Cloud/Demo",
        label: "Demo",
        hosts: [
          {
            name: "web-01",
            alias: "gcp_demo_web-01",
            synced: "gcp",
            destination: "ubuntu@34.123.45.67",
            summary: "GCP synced · known host",
            rows: [
              { label: "Source", value: "~/.ssh/bast/sync/gcp/config:1" },
              { label: "SSH name", value: "gcp_demo_web-01" },
              { label: "Auth", value: "ubuntu · ~/.ssh/bast/keys/work" },
              { label: "Used", value: "2026-07-26 13:40 (4)" },
            ],
          },
          {
            name: "gpu-training",
            alias: "gcp_demo_gpu-training",
            synced: "gcp",
            destination: "gpu-training",
            summary: "GCP synced · known host",
            rows: [
              { label: "Source", value: "~/.ssh/bast/sync/gcp/config:14" },
              { label: "SSH name", value: "gcp_demo_gpu-training" },
              {
                label: "Auth",
                value: "oslogin_user · ~/.ssh/google_compute_engine",
              },
              { label: "Jump", value: "IAP tunnel" },
            ],
          },
        ],
      },
    ],
  },
  {
    path: "Amazon EC2",
    label: "Amazon EC2",
    synced: true,
    children: [
      {
        path: "Amazon EC2/production/us-east-1",
        label: "production / us-east-1",
        hosts: [
          {
            name: "worker-01",
            alias: "aws_production_us-east-1_i-0123456789abcdef0",
            synced: "aws",
            destination: "ec2-user@10.20.1.14",
            summary: "AWS synced · known host",
            rows: [
              { label: "Source", value: "~/.ssh/bast/sync/aws/config:1" },
              { label: "SSH name", value: "aws_production_us-east-1_i-0123456789abcdef0" },
              { label: "Jump", value: "EC2 Instance Connect Endpoint" },
            ],
          },
        ],
      },
    ],
  },
  {
    path: "Microsoft Azure",
    label: "Microsoft Azure",
    synced: true,
    children: [
      {
        path: "Microsoft Azure/Production/apps",
        label: "Production / apps",
        hosts: [
          {
            name: "api-01",
            alias: "azure_Production_apps_api-01",
            synced: "azure",
            destination: "azureuser@10.30.2.8",
            summary: "Azure synced · known host",
            rows: [
              { label: "Source", value: "~/.ssh/bast/sync/azure/config:1" },
              { label: "SSH name", value: "azure_Production_apps_api-01" },
              { label: "Jump", value: "Azure Bastion tunnel" },
            ],
          },
        ],
      },
    ],
  },
];

function groupCount(node: HostGroupNode): number {
  const own = node.hosts?.length ?? 0;
  const nested = (node.children ?? []).reduce(
    (sum, child) => sum + groupCount(child),
    0,
  );
  return own + nested;
}

const ungroupedHosts: HostItem[] = [
  {
    name: "github.com",
    destination: "git@github.com",
    summary: "external · known host",
    rows: [
      { label: "Source", value: "~/.ssh/config:92" },
      { label: "Key", value: "~/.ssh/id_ed25519_github" },
    ],
  },
];

const keys: KeyItem[] = [
  {
    name: "work",
    inAgent: true,
    summary: "ed25519 · Bast managed · agent cached",
    fingerprint: "SHA256:abc123…bast",
    rows: [
      { label: "Private", value: "~/.ssh/bast/keys/work" },
      { label: "Public", value: "~/.ssh/bast/keys/work.pub" },
      { label: "Used by", value: "api, bastion, web-01" },
    ],
  },
  {
    name: "id_rsa_legacy",
    summary: "rsa · external",
    fingerprint: "SHA256:def456…legacy",
    rows: [
      { label: "Private", value: "~/.ssh/id_rsa" },
      { label: "Public", value: "~/.ssh/id_rsa.pub" },
    ],
  },
  {
    name: "github_deploy",
    summary: "ed25519 · external · agent cached",
    inAgent: true,
    fingerprint: "SHA256:ghi789…deploy",
    rows: [
      { label: "Private", value: "~/.ssh/id_ed25519_github" },
      { label: "Used by", value: "github.com" },
    ],
  },
];

const syncProviders: SyncMenuItem[] = [
  { label: "GCP", detail: "2 · 2026-07-26 13:55", action: "open-gcp" },
  { label: "AWS", detail: "4 · 2026-07-27 18:20", action: "open-aws" },
  { label: "Azure", detail: "3 · 2026-07-28 10:12", action: "open-azure" },
];

const syncGcpActions: SyncMenuItem[] = [
  { label: "Sync now" },
  { label: "Disconnect" },
  { label: "Disable auto-sync" },
  { label: "Default SSH user" },
  { label: "Project filter" },
  { label: "Add service account key" },
  { label: "Refresh status" },
];

const gcpStatusRows: DetailRow[] = [
  { label: "Status", value: "enabled" },
  { label: "Accounts", value: "you@example.com" },
  { label: "Last sync", value: "2026-07-26 13:55 · 2" },
  { label: "Auto-sync", value: "on" },
  { label: "SSH user", value: "none" },
  { label: "Projects", value: "all" },
];

const syncProviderActions: Record<Exclude<SyncProvider, "">, SyncMenuItem[]> = {
  gcp: syncGcpActions,
  aws: [
    { label: "Sync now" },
    { label: "Disconnect" },
    { label: "Enable auto-sync" },
    { label: "Default SSH user" },
    { label: "Profile filter" },
    { label: "Region filter" },
    { label: "Refresh status" },
  ],
  azure: [
    { label: "Sync now" },
    { label: "Disconnect" },
    { label: "Enable auto-sync" },
    { label: "Default SSH user" },
    { label: "Subscription filter" },
    { label: "Resource group filter" },
    { label: "Refresh status" },
  ],
};

const syncProviderStatus: Record<Exclude<SyncProvider, "">, DetailRow[]> = {
  gcp: gcpStatusRows,
  aws: [
    { label: "Status", value: "enabled" },
    { label: "Profiles", value: "default, production" },
    { label: "Last sync", value: "2026-07-27 18:20 · 4" },
    { label: "Auto-sync", value: "off" },
    { label: "Regions", value: "all enabled" },
  ],
  azure: [
    { label: "Status", value: "enabled" },
    { label: "Subscriptions", value: "Production" },
    { label: "Last sync", value: "2026-07-28 10:12 · 3" },
    { label: "Auto-sync", value: "off" },
    { label: "Resource groups", value: "all" },
  ],
};

function providerForAction(
  action: SyncMenuItem["action"],
): Exclude<SyncProvider, ""> | null {
  switch (action) {
    case "open-gcp":
      return "gcp";
    case "open-aws":
      return "aws";
    case "open-azure":
      return "azure";
    default:
      return null;
  }
}

const panelGrid = "grid h-full min-h-0 grid-cols-[38%_1fr]";

const bleedX =
  "-left-3 -right-3 sm:-left-4 sm:-right-4 md:-left-5 md:-right-5";

const bleedMargin = "-mx-3 sm:-mx-4";

function appendGroupRows(
  rows: HostListRow[],
  node: HostGroupNode,
  depth: number,
  collapsed: Record<string, boolean>,
  parentSynced = false,
): HostListRow[] {
  const synced = Boolean(parentSynced || node.synced || node.googleCloud);
  rows.push({
    kind: "group",
    id: node.path,
    label: node.label,
    depth,
    count: groupCount(node),
    googleCloud: node.googleCloud,
    synced,
  });
  if (collapsed[node.path]) {
    return rows;
  }
  for (const host of node.hosts ?? []) {
    rows.push({
      kind: "host",
      id: host.alias ?? host.name,
      host,
      depth: depth + 1,
    });
  }
  for (const child of node.children ?? []) {
    appendGroupRows(rows, child, depth + 1, collapsed, synced);
  }
  return rows;
}

function buildHostRows(collapsed: Record<string, boolean>): HostListRow[] {
  const rows: HostListRow[] = [];
  for (const node of hostTree) {
    appendGroupRows(rows, node, 0, collapsed);
  }
  for (const host of ungroupedHosts) {
    rows.push({
      kind: "host",
      id: host.alias ?? host.name,
      host,
      depth: 0,
    });
  }
  return rows;
}

function defaultHostCursor(rows: HostListRow[]): number {
  const gcp = rows.findIndex(
    (row) => row.kind === "host" && row.host.synced === "gcp",
  );
  return gcp >= 0 ? gcp : 0;
}

export function TuiDemo() {
  const [section, setSection] = useState<Section>("hosts");
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const [hostCursor, setHostCursor] = useState(() =>
    defaultHostCursor(buildHostRows({})),
  );
  const [keyCursor, setKeyCursor] = useState(0);
  const [syncProvider, setSyncProvider] = useState<SyncProvider>("");
  const [syncCursor, setSyncCursor] = useState(0);
  const containerRef = useRef<HTMLDivElement>(null);

  const hostRows = useMemo(() => buildHostRows(collapsed), [collapsed]);
  const safeHostCursor = Math.min(hostCursor, Math.max(0, hostRows.length - 1));
  const selectedHostRow = hostRows[safeHostCursor];
  const selectedKey = keys[Math.min(keyCursor, keys.length - 1)];

  const syncItems = syncProvider
    ? syncProviderActions[syncProvider]
    : syncProviders;
  const safeSyncCursor = Math.min(syncCursor, Math.max(0, syncItems.length - 1));

  const switchSection = useCallback((next: Section) => {
    setSection(next);
    if (next === "sync") {
      setSyncProvider("");
      setSyncCursor(0);
    }
  }, []);

  const toggleGroup = useCallback((groupId: string) => {
    setCollapsed((current) => ({ ...current, [groupId]: !current[groupId] }));
  }, []);

  const moveHosts = useCallback(
    (delta: number) => {
      setHostCursor((current) =>
        Math.max(0, Math.min(hostRows.length - 1, current + delta)),
      );
    },
    [hostRows.length],
  );

  const moveKeys = useCallback((delta: number) => {
    setKeyCursor((current) =>
      Math.max(0, Math.min(keys.length - 1, current + delta)),
    );
  }, []);

  const moveSync = useCallback(
    (delta: number) => {
      setSyncCursor((current) => {
        let next = current;
        for (let step = 0; step < syncItems.length; step += 1) {
          next = Math.max(0, Math.min(syncItems.length - 1, next + delta));
          if (!syncItems[next]?.disabled) {
            return next;
          }
        }
        return current;
      });
    },
    [syncItems],
  );

  const activateSync = useCallback(() => {
    const item = syncItems[safeSyncCursor];
    if (!item || item.disabled) return;
    const provider = providerForAction(item.action);
    if (provider) {
      setSyncProvider(provider);
      setSyncCursor(0);
    }
  }, [safeSyncCursor, syncItems]);

  const onKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (
        (event.key === "Enter" || event.key === " ") &&
        event.target !== event.currentTarget &&
        (event.target as HTMLElement).closest(
          "button, a, input, select, textarea, [role='button']",
        )
      ) {
        return;
      }
      switch (event.key) {
        case "j":
        case "ArrowDown":
          event.preventDefault();
          if (section === "hosts") moveHosts(1);
          else if (section === "keys") moveKeys(1);
          else moveSync(1);
          break;
        case "k":
        case "ArrowUp":
          event.preventDefault();
          if (section === "hosts") moveHosts(-1);
          else if (section === "keys") moveKeys(-1);
          else moveSync(-1);
          break;
        case "1":
          event.preventDefault();
          switchSection("hosts");
          break;
        case "2":
          event.preventDefault();
          switchSection("keys");
          break;
        case "3":
          event.preventDefault();
          switchSection("sync");
          break;
        case "Enter":
        case " ":
          event.preventDefault();
          if (section === "hosts" && selectedHostRow?.kind === "group") {
            toggleGroup(selectedHostRow.id);
          } else if (section === "sync") {
            activateSync();
          }
          break;
        case "Escape":
          if (section === "sync" && syncProvider) {
            event.preventDefault();
            setSyncProvider("");
            setSyncCursor(0);
          }
          break;
      }
    },
    [
      activateSync,
      moveHosts,
      moveKeys,
      moveSync,
      section,
      selectedHostRow,
      switchSection,
      syncProvider,
      toggleGroup,
    ],
  );

  const footerHintDesktop =
    section === "hosts"
      ? "↵ connect • ␣ group • a add • e edit • 3 sync • ? help"
      : section === "keys"
        ? "a generate • i import • x export • u add to server • ? help"
        : syncProvider
          ? "↵ action • esc providers • r refresh • ? help"
          : "↵ open provider • 1 hosts • 2 keys • ? help";

  const footerHintMobile =
    section === "hosts"
      ? "tap select • 1/2/3 tabs"
      : section === "keys"
        ? "tap select • 1/2/3 tabs"
        : "tap provider • 1/2/3 tabs";

  return (
    <div
      ref={containerRef}
      tabIndex={0}
      onKeyDown={onKeyDown}
      onMouseDown={() => containerRef.current?.focus()}
      className="absolute inset-0 flex flex-col overflow-hidden p-3 pt-2 font-mono text-xs leading-snug text-foreground outline-none focus-visible:ring-1 focus-visible:ring-border focus-visible:ring-inset sm:p-4 sm:pt-2 sm:text-sm md:px-5 md:pt-2 md:pb-0 md:text-[15px]"
      role="application"
      aria-label="Interactive Bast TUI demo. Use j/k or arrow keys to navigate, 1/2/3 to switch tabs."
    >
      <header className="mb-2 flex shrink-0 items-center gap-x-2 sm:gap-x-3">
        <span className="bg-accent px-1.5 py-0.5 text-[10px] font-bold tracking-wide text-white sm:text-xs md:text-sm">
          {" BAST "}
        </span>
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 text-[11px] sm:gap-x-3 sm:text-inherit">
          <TabButton
            active={section === "hosts"}
            onClick={() => switchSection("hosts")}
          >
            [1] Hosts
          </TabButton>
          <TabButton
            active={section === "keys"}
            onClick={() => switchSection("keys")}
          >
            [2] Keys
          </TabButton>
          <TabButton
            active={section === "sync"}
            onClick={() => switchSection("sync")}
          >
            [3] Sync
          </TabButton>
        </span>
        <span className="ml-auto text-[11px] font-bold text-muted sm:text-sm md:text-[15px]">
          Demo
        </span>
      </header>

      {section === "sync" ? (
        <SyncPanel
          provider={syncProvider}
          items={syncItems}
          cursor={safeSyncCursor}
          onSelect={(index) => {
            setSyncCursor(index);
            const item = syncItems[index];
            const provider = providerForAction(item?.action);
            if (provider) {
              setSyncProvider(provider);
              setSyncCursor(0);
            }
          }}
          onBack={() => {
            setSyncProvider("");
            setSyncCursor(0);
          }}
          footerDesktop={footerHintDesktop}
          footerMobile={footerHintMobile}
        />
      ) : (
        <>
          <div className="flex min-h-0 flex-1 flex-col md:hidden">
            <div className={`h-px shrink-0 bg-border ${bleedMargin}`} />
            {section === "hosts" ? (
              <HostList
                rows={hostRows}
                cursor={safeHostCursor}
                collapsed={collapsed}
                onSelect={setHostCursor}
                onToggleGroup={toggleGroup}
                className="shrink-0 pt-1.5"
              />
            ) : (
              <KeyList
                items={keys}
                cursor={keyCursor}
                onSelect={setKeyCursor}
                className="shrink-0 pt-1.5"
              />
            )}
            <div className={`my-1.5 h-px shrink-0 bg-border ${bleedMargin}`} />
            <div className="min-h-0 flex-1 overflow-hidden pt-0.5">
              {section === "hosts" ? (
                <HostDetailPane row={selectedHostRow} collapsed={collapsed} compact />
              ) : (
                <KeyDetail keyItem={selectedKey} compact />
              )}
            </div>
          </div>

          <div className="relative hidden min-h-0 flex-1 md:block">
            <div
              aria-hidden
              className={`pointer-events-none absolute top-0 h-px bg-border ${bleedX}`}
            />

            <div className={`${panelGrid} md:grid`}>
              {section === "hosts" ? (
                <HostList
                  rows={hostRows}
                  cursor={safeHostCursor}
                  collapsed={collapsed}
                  onSelect={setHostCursor}
                  onToggleGroup={toggleGroup}
                  className="min-h-0 overflow-hidden pt-1.5"
                />
              ) : (
                <KeyList
                  items={keys}
                  cursor={keyCursor}
                  onSelect={setKeyCursor}
                  className="min-h-0 overflow-hidden pt-1.5"
                />
              )}

              <div className="min-w-0 overflow-hidden pt-1.5 pb-10 pl-3 md:pl-4">
                {section === "hosts" ? (
                  <HostDetailPane row={selectedHostRow} collapsed={collapsed} />
                ) : (
                  <KeyDetail keyItem={selectedKey} />
                )}
              </div>
            </div>

            <div
              aria-hidden
              className="pointer-events-none absolute top-0 bottom-0 left-[38%] z-10 w-px bg-border"
            />

            <footer className="pointer-events-none absolute inset-x-0 bottom-0 truncate pb-5 pl-[calc(38%+0.75rem)] text-right text-[10px] text-muted sm:text-xs md:pl-[calc(38%+1rem)] md:text-sm">
              {footerHintDesktop}
            </footer>
          </div>

          <footer className="mt-2 shrink-0 truncate text-right text-[10px] text-muted sm:text-xs md:hidden md:text-sm">
            {footerHintMobile}
          </footer>
        </>
      )}
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`cursor-pointer border-0 bg-transparent p-0 font-[inherit] font-bold transition-colors ${
        active ? "text-accent" : "text-muted hover:text-foreground"
      }`}
    >
      {children}
    </button>
  );
}

function HostList({
  rows,
  cursor,
  collapsed,
  onSelect,
  onToggleGroup,
  className,
}: {
  rows: HostListRow[];
  cursor: number;
  collapsed: Record<string, boolean>;
  onSelect: (index: number) => void;
  onToggleGroup: (groupId: string) => void;
  className?: string;
}) {
  return (
    <ul className={className} role="listbox" aria-label="hosts">
      {rows.map((row, index) => {
        const isSelected = index === cursor;
        const pad = row.depth > 0 ? { paddingLeft: `${row.depth * 0.95}rem` } : undefined;
        if (row.kind === "group") {
          const indicator = collapsed[row.id] ? "▸" : "▾";
          return (
            <li key={row.id} role="option" aria-selected={isSelected}>
              <button
                type="button"
                onClick={() => {
                  onSelect(index);
                  onToggleGroup(row.id);
                }}
                style={pad}
                className={`w-full cursor-pointer truncate whitespace-nowrap border-0 py-0.5 text-left font-[inherit] font-bold transition-colors ${
                  isSelected
                    ? "bg-highlight text-foreground"
                    : "bg-transparent text-accent hover:bg-highlight/70"
                }`}
              >
                {indicator}{" "}
                {row.googleCloud ? <GoogleCloudLabel /> : row.label}{" "}
                <span className="font-normal text-muted">({row.count})</span>
              </button>
            </li>
          );
        }

        const prefix = row.host.favorite ? "◆ " : "  ";
        return (
          <li key={row.id} role="option" aria-selected={isSelected}>
            <button
              type="button"
              onClick={() => onSelect(index)}
              style={pad}
              className={`w-full cursor-pointer truncate whitespace-pre border-0 py-0.5 text-left font-[inherit] transition-colors ${
                isSelected
                  ? "bg-highlight font-bold text-foreground"
                  : "bg-transparent text-foreground hover:bg-highlight/70"
              }`}
            >
              {prefix}
              {row.host.name}
            </button>
          </li>
        );
      })}
    </ul>
  );
}

function KeyList({
  items,
  cursor,
  onSelect,
  className,
}: {
  items: KeyItem[];
  cursor: number;
  onSelect: (index: number) => void;
  className?: string;
}) {
  return (
    <ul className={className} role="listbox" aria-label="keys">
      {items.map((item, index) => {
        const isSelected = index === cursor;
        const prefix = item.inAgent ? "● " : "  ";
        return (
          <li key={item.name} role="option" aria-selected={isSelected}>
            <button
              type="button"
              onClick={() => onSelect(index)}
              className={`w-full cursor-pointer truncate whitespace-nowrap border-0 py-0.5 text-left font-[inherit] transition-colors ${
                isSelected
                  ? "bg-highlight font-bold text-foreground"
                  : "bg-transparent text-foreground hover:bg-highlight/70"
              }`}
            >
              {prefix}
              {item.name}
            </button>
          </li>
        );
      })}
    </ul>
  );
}

function HostDetailPane({
  row,
  collapsed,
  compact = false,
}: {
  row: HostListRow | undefined;
  collapsed: Record<string, boolean>;
  compact?: boolean;
}) {
  if (!row) return null;
  if (row.kind === "group") {
    const state = collapsed[row.id] ? "collapsed" : "expanded";
    const hint = row.synced
      ? "Press ␣ to collapse or expand · cloud sync group (read-only)"
      : "Press ␣ to collapse or expand · e to rename";
    return (
      <>
        <p className="truncate font-bold text-accent">
          {row.googleCloud ? <GoogleCloudLabel /> : row.label}
        </p>
        <p className="truncate text-muted">
          {row.count} servers · {state}
        </p>
        <p className={`truncate ${compact ? "mt-1.5" : "mt-2"}`}>{hint}</p>
      </>
    );
  }
  return <HostDetail host={row.host} compact={compact} />;
}

function HostDetail({
  host,
  compact = false,
}: {
  host: HostItem;
  compact?: boolean;
}) {
  return (
    <>
      <div className="flex items-start justify-between gap-2">
        <p className="min-w-0 truncate font-bold text-accent">{host.name}</p>
        <span className="shrink-0 bg-accent px-1.5 py-0.5 text-[10px] font-bold tracking-wide text-white sm:text-xs">
          {" Connect "}
        </span>
      </div>
      <p className="truncate">{host.destination}</p>
      <p className="truncate text-muted">{host.summary}</p>
      <div className={`space-y-0.5 ${compact ? "mt-1.5" : "mt-2"}`}>
        {host.rows.map((row) => (
          <DetailLine key={row.label} {...row} compact={compact} />
        ))}
      </div>
    </>
  );
}

function KeyDetail({
  keyItem,
  compact = false,
}: {
  keyItem: KeyItem;
  compact?: boolean;
}) {
  return (
    <>
      <p className="truncate font-bold text-accent">{keyItem.name}</p>
      <p className="truncate text-muted">{keyItem.summary}</p>
      {keyItem.fingerprint ? (
        <p className="truncate">{keyItem.fingerprint}</p>
      ) : null}
      <div className={`space-y-0.5 ${compact ? "mt-1.5" : "mt-2"}`}>
        <p className={`font-bold text-accent ${compact ? "mt-1" : "mt-1.5"}`}>
          [u] Add to server
        </p>
        {keyItem.rows.map((row) => (
          <DetailLine key={row.label} {...row} compact={compact} />
        ))}
      </div>
    </>
  );
}

function SyncPanel({
  provider,
  items,
  cursor,
  onSelect,
  onBack,
  footerDesktop,
  footerMobile,
}: {
  provider: SyncProvider;
  items: SyncMenuItem[];
  cursor: number;
  onSelect: (index: number) => void;
  onBack: () => void;
  footerDesktop: string;
  footerMobile: string;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className={`h-px shrink-0 bg-border ${bleedMargin}`} />
      <div className="min-h-0 flex-1 overflow-hidden pt-2">
        {provider === "" ? (
          <>
            <p className="font-bold text-accent">Providers</p>
            <p className="mb-2 text-muted">
              Browse and connect to VMs from your cloud accounts.
            </p>
          </>
        ) : (
          <>
            <div className="mb-1 flex items-center justify-between gap-2">
              <p className="font-bold text-accent">
                {provider === "gcp" ? "GCP" : provider === "aws" ? "AWS" : "Azure"}
              </p>
              <button
                type="button"
                onClick={onBack}
                className="cursor-pointer border-0 bg-transparent p-0 font-[inherit] text-muted hover:text-foreground"
              >
                esc
              </button>
            </div>
            <div className="mb-3 space-y-0.5">
              {syncProviderStatus[provider].map((row) => (
                <DetailLine key={row.label} {...row} />
              ))}
            </div>
          </>
        )}
        <ul role="listbox" aria-label="sync" className="max-w-md">
          {items.map((item, index) => {
            const isSelected = index === cursor && !item.disabled;
            return (
              <li key={item.label} role="option" aria-selected={isSelected}>
                <button
                  type="button"
                  disabled={item.disabled}
                  onClick={() => onSelect(index)}
                  className={`flex w-full items-baseline gap-4 border-0 py-0.5 text-left font-[inherit] transition-colors ${
                    item.disabled
                      ? "cursor-default bg-transparent text-muted"
                      : isSelected
                        ? "cursor-pointer bg-highlight font-bold text-foreground"
                        : "cursor-pointer bg-transparent text-foreground hover:bg-highlight/70"
                  }`}
                >
                  <span className="shrink-0 pl-2">{item.label}</span>
                  {item.detail ? (
                    <span className="min-w-0 flex-1 truncate text-right">
                      {item.detail}
                    </span>
                  ) : null}
                </button>
              </li>
            );
          })}
        </ul>
      </div>
      <footer className="mt-2 hidden shrink-0 truncate text-right text-[10px] text-muted sm:text-xs md:block md:text-sm">
        {footerDesktop}
      </footer>
      <footer className="mt-2 shrink-0 truncate text-right text-[10px] text-muted sm:text-xs md:hidden">
        {footerMobile}
      </footer>
    </div>
  );
}

function GoogleCloudLabel() {
  return (
    <>
      <span className="font-bold">
        <span style={{ color: "#4285F4" }}>G</span>
        <span style={{ color: "#EA4335" }}>o</span>
        <span style={{ color: "#FBBC05" }}>o</span>
        <span style={{ color: "#4285F4" }}>g</span>
        <span style={{ color: "#34A853" }}>l</span>
        <span style={{ color: "#EA4335" }}>e</span>
      </span>
      <span className="font-bold text-foreground"> Cloud</span>
    </>
  );
}

function DetailLine({
  label,
  value,
  compact = false,
}: DetailRow & { compact?: boolean }) {
  if (compact) {
    return (
      <div className="min-w-0">
        <span className="text-[10px] text-muted sm:text-[11px]">{label}</span>
        <p className="truncate">{value}</p>
      </div>
    );
  }

  return (
    <p className="flex min-w-0 items-baseline gap-3 truncate">
      <span className="inline-block w-[10ch] shrink-0 text-muted sm:w-[12ch]">
        {label}
      </span>
      <span className="min-w-0 truncate">{value}</span>
    </p>
  );
}
