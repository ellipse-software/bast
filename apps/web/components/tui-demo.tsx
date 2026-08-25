"use client";

import { useCallback, useMemo, useRef, useState, type ReactNode } from "react";

type Section = "hosts" | "keys" | "vault" | "sync" | "files";
type SyncProvider = "" | "gcp" | "aws" | "azure" | "box" | "upstash";

type DetailRow = { label: string; value: string };

type FileEntry = {
  name: string;
  isDir?: boolean;
  mode: string;
  symbolic: string;
  size: string;
  modified: string;
};

type HostItem = {
  name: string;
  alias?: string;
  favorite?: boolean;
  synced?: "gcp" | "aws" | "azure";
  destination: string;
  summary: string;
  access?: DetailRow[];
  about?: DetailRow[];
  promote?: boolean;
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
  description?: string;
  disabled?: boolean;
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
        access: [{ label: "Auth", value: "~/.ssh/bast/keys/work" }],
        about: [
          { label: "Tags", value: "web, api" },
          { label: "Used", value: "2026-07-26 14:02 (18)" },
        ],
      },
      {
        name: "bastion",
        destination: "ops@bastion.prod.example.com",
        summary: "Bast managed · known host",
        access: [{ label: "Auth", value: "~/.ssh/bast/keys/work" }],
        about: [{ label: "Tags", value: "jump" }],
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
            access: [
              { label: "Auth", value: "~/.ssh/bast/keys/work" },
              { label: "SSH name", value: "gcp_demo_web-01" },
            ],
            about: [{ label: "Used", value: "2026-07-26 13:40 (4)" }],
          },
          {
            name: "gpu-training",
            alias: "gcp_demo_gpu-training",
            synced: "gcp",
            destination: "gpu-training",
            summary: "GCP synced · known host",
            access: [
              {
                label: "Auth",
                value: "oslogin_user · ~/.ssh/google_compute_engine",
              },
              { label: "Jump", value: "IAP tunnel" },
              { label: "SSH name", value: "gcp_demo_gpu-training" },
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
            access: [
              { label: "Jump", value: "EC2 Instance Connect Endpoint" },
              {
                label: "SSH name",
                value: "aws_production_us-east-1_i-0123456789abcdef0",
              },
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
            access: [
              { label: "Jump", value: "Azure Bastion tunnel" },
              { label: "SSH name", value: "azure_Production_apps_api-01" },
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
    access: [{ label: "Auth", value: "~/.ssh/id_ed25519_github" }],
    promote: true,
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

type SyncTile = {
  id: Exclude<SyncProvider, "">;
  title: string;
  googleCloud?: boolean;
  detail: string;
  description: string;
};

const syncTiles: SyncTile[] = [
  {
    id: "gcp",
    title: "Google Cloud",
    googleCloud: true,
    detail: "2 · 2026-07-26 13:55",
    description: "Import Compute Engine VMs into Bast",
  },
  {
    id: "aws",
    title: "Amazon EC2",
    detail: "4 · 2026-07-27 18:20",
    description: "Import EC2 instances into Bast",
  },
  {
    id: "azure",
    title: "Microsoft Azure",
    detail: "3 · 2026-07-28 10:12",
    description: "Import Azure VMs into Bast",
  },
  {
    id: "box",
    title: "Box",
    detail: "2 running",
    description: "Import ASCII Box sandboxes into Bast",
  },
  {
    id: "upstash",
    title: "Upstash",
    detail: "2 running",
    description: "Import Upstash Box sandboxes into Bast",
  },
];

const vaultMenu: SyncMenuItem[] = [
  {
    label: "Rotate passphrase",
    description: "Requires the current passphrase",
  },
  {
    label: "Reset passphrase",
    description: "Overwrites the remote vault with this machine",
  },
  {
    label: "API base URL",
    description: "Vault server for this machine (self-hosted or bast.sh)",
  },
  { label: "Log out" },
];

type ProviderPage = {
  identity: string[];
  chips: string[];
  config: SyncMenuItem[];
};

const providerPages: Record<Exclude<SyncProvider, "">, ProviderPage> = {
  gcp: {
    identity: [
      "enabled · 2 · 2026-07-26 13:55",
      "you@example.com · auto-sync on",
    ],
    chips: ["Sync"],
    config: [
      { label: "Disconnect" },
      { label: "Disable auto-sync" },
      { label: "Default SSH user" },
      { label: "Project filter" },
      { label: "Add service account key" },
      { label: "Refresh status" },
    ],
  },
  aws: {
    identity: [
      "enabled · 4 · 2026-07-27 18:20",
      "default, production · auto-sync off",
    ],
    chips: ["Sync"],
    config: [
      { label: "Disconnect" },
      { label: "Enable auto-sync" },
      { label: "Default SSH user" },
      { label: "Profile filter" },
      { label: "Region filter" },
      { label: "Refresh status" },
    ],
  },
  azure: {
    identity: [
      "enabled · 3 · 2026-07-28 10:12",
      "Production · auto-sync off",
    ],
    chips: ["Sync"],
    config: [
      { label: "Disconnect" },
      { label: "Enable auto-sync" },
      { label: "Default SSH user" },
      { label: "Subscription filter" },
      { label: "Resource group filter" },
      { label: "Refresh status" },
    ],
  },
  box: {
    identity: ["enabled · 2 running", "ted · auto-sync on"],
    chips: ["Sync", "New box"],
    config: [
      { label: "Disconnect" },
      { label: "Refresh status" },
    ],
  },
  upstash: {
    identity: ["enabled · 2 running", "key stored · auto-sync on"],
    chips: ["Sync", "New box", "API key"],
    config: [
      { label: "Disconnect" },
      { label: "Disable auto-sync" },
      { label: "Refresh status" },
    ],
  },
};

const localFiles: FileEntry[] = [
  {
    name: "Documents",
    isDir: true,
    mode: "0755",
    symbolic: "rwxr-xr-x",
    size: "-",
    modified: "2026-07-20 09:12",
  },
  {
    name: "deploy",
    isDir: true,
    mode: "0755",
    symbolic: "rwxr-xr-x",
    size: "-",
    modified: "2026-07-26 14:02",
  },
  {
    name: "notes.md",
    mode: "0644",
    symbolic: "rw-r--r--",
    size: "2.1 KB",
    modified: "2026-07-25 11:40",
  },
  {
    name: "secret.env",
    mode: "0600",
    symbolic: "rw-------",
    size: "128 B",
    modified: "2026-07-26 13:55",
  },
  {
    name: "id_ed25519",
    mode: "0600",
    symbolic: "rw-------",
    size: "419 B",
    modified: "2026-06-02 08:18",
  },
  {
    name: "id_ed25519.pub",
    mode: "0644",
    symbolic: "rw-r--r--",
    size: "104 B",
    modified: "2026-06-02 08:18",
  },
];

const remoteFiles: FileEntry[] = [
  {
    name: "app",
    isDir: true,
    mode: "0755",
    symbolic: "rwxr-xr-x",
    size: "-",
    modified: "2026-07-26 12:01",
  },
  {
    name: "bin",
    isDir: true,
    mode: "0755",
    symbolic: "rwxr-xr-x",
    size: "-",
    modified: "2026-07-10 16:22",
  },
  {
    name: "etc",
    isDir: true,
    mode: "0755",
    symbolic: "rwxr-xr-x",
    size: "-",
    modified: "2026-07-01 08:00",
  },
  {
    name: "home",
    isDir: true,
    mode: "0755",
    symbolic: "rwxr-xr-x",
    size: "-",
    modified: "2026-07-18 19:44",
  },
  {
    name: "var",
    isDir: true,
    mode: "0755",
    symbolic: "rwxr-xr-x",
    size: "-",
    modified: "2026-07-22 03:11",
  },
  {
    name: "README",
    mode: "0644",
    symbolic: "rw-r--r--",
    size: "1.4 KB",
    modified: "2026-07-26 12:02",
  },
  {
    name: "docker-compose.yml",
    mode: "0644",
    symbolic: "rw-r--r--",
    size: "980 B",
    modified: "2026-07-26 12:05",
  },
];

const localTone = "#94A3B8";
const remoteTone = "#14B8A6";

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
  const [vaultCursor, setVaultCursor] = useState(-1);
  const [filesFocus, setFilesFocus] = useState<0 | 1>(0);
  const [localCursor, setLocalCursor] = useState(2);
  const [remoteCursor, setRemoteCursor] = useState(0);
  const [localMarked, setLocalMarked] = useState<Record<string, boolean>>({
    "secret.env": true,
  });
  const [filesInfo, setFilesInfo] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  const hostRows = useMemo(() => buildHostRows(collapsed), [collapsed]);
  const safeHostCursor = Math.min(hostCursor, Math.max(0, hostRows.length - 1));
  const selectedHostRow = hostRows[safeHostCursor];
  const selectedKey = keys[Math.min(keyCursor, keys.length - 1)];

  const providerPage = syncProvider ? providerPages[syncProvider] : null;
  const syncItemCount = providerPage
    ? providerPage.chips.length + providerPage.config.length
    : syncTiles.length;
  const safeSyncCursor = Math.min(syncCursor, Math.max(0, syncItemCount - 1));
  const safeVaultCursor = Math.min(vaultCursor, vaultMenu.length - 1);
  const safeLocalCursor = Math.min(
    localCursor,
    Math.max(0, localFiles.length - 1),
  );
  const safeRemoteCursor = Math.min(
    remoteCursor,
    Math.max(0, remoteFiles.length - 1),
  );

  const switchSection = useCallback((next: Section) => {
    setSection(next);
    if (next === "sync") {
      setSyncProvider("");
      setSyncCursor(0);
    }
    if (next === "vault") {
      setVaultCursor(-1);
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
        if (!providerPage) {
          return Math.max(0, Math.min(syncItemCount - 1, current + delta));
        }
        const chipCount = providerPage.chips.length;
        if (delta > 0 && current < chipCount) {
          return Math.min(syncItemCount - 1, chipCount);
        }
        if (delta < 0 && current === chipCount) {
          return 0;
        }
        return Math.max(0, Math.min(syncItemCount - 1, current + delta));
      });
    },
    [providerPage, syncItemCount],
  );

  const moveVault = useCallback((delta: number) => {
    setVaultCursor((current) =>
      Math.max(-1, Math.min(vaultMenu.length - 1, current + delta)),
    );
  }, []);

  const moveFiles = useCallback(
    (delta: number) => {
      if (filesFocus === 0) {
        setLocalCursor((current) =>
          Math.max(0, Math.min(localFiles.length - 1, current + delta)),
        );
      } else {
        setRemoteCursor((current) =>
          Math.max(0, Math.min(remoteFiles.length - 1, current + delta)),
        );
      }
    },
    [filesFocus],
  );

  const toggleFilesMark = useCallback(() => {
    if (filesFocus !== 0) return;
    const entry = localFiles[safeLocalCursor];
    if (!entry) return;
    setLocalMarked((current) => {
      const next = { ...current };
      if (next[entry.name]) {
        delete next[entry.name];
      } else {
        next[entry.name] = true;
      }
      return next;
    });
  }, [filesFocus, safeLocalCursor]);

  const activateSync = useCallback(() => {
    if (syncProvider) return;
    const tile = syncTiles[safeSyncCursor];
    if (!tile) return;
    setSyncProvider(tile.id);
    setSyncCursor(0);
  }, [safeSyncCursor, syncProvider]);

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
          else if (section === "files") moveFiles(1);
          else if (section === "vault") moveVault(1);
          else moveSync(1);
          break;
        case "k":
        case "ArrowUp":
          event.preventDefault();
          if (section === "hosts") moveHosts(-1);
          else if (section === "keys") moveKeys(-1);
          else if (section === "files") moveFiles(-1);
          else if (section === "vault") moveVault(-1);
          else moveSync(-1);
          break;
        case "h":
        case "ArrowLeft":
          if (section === "sync" && !syncProvider) {
            event.preventDefault();
            moveSync(-1);
          } else if (section === "sync" && providerPage && safeSyncCursor > 0 && safeSyncCursor < providerPage.chips.length) {
            event.preventDefault();
            moveSync(-1);
          }
          break;
        case "l":
        case "ArrowRight":
          if (section === "sync" && !syncProvider) {
            event.preventDefault();
            moveSync(1);
          } else if (section === "sync" && providerPage && safeSyncCursor < providerPage.chips.length - 1) {
            event.preventDefault();
            moveSync(1);
          }
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
          switchSection("vault");
          break;
        case "4":
          event.preventDefault();
          switchSection("sync");
          break;
        case "5":
          event.preventDefault();
          switchSection("files");
          break;
        case "Tab":
          if (section === "files") {
            event.preventDefault();
            setFilesFocus((current) => (current === 0 ? 1 : 0));
          }
          break;
        case "i":
          if (section === "files") {
            event.preventDefault();
            setFilesInfo((current) => !current);
          }
          break;
        case "Enter":
        case " ":
          event.preventDefault();
          if (section === "hosts" && selectedHostRow?.kind === "group") {
            toggleGroup(selectedHostRow.id);
          } else if (section === "sync") {
            activateSync();
          } else if (section === "files" && event.key === " ") {
            toggleFilesMark();
          }
          break;
        case "Escape":
          if (section === "sync" && syncProvider) {
            event.preventDefault();
            setSyncProvider("");
            setSyncCursor(0);
          } else if (section === "files" && filesInfo) {
            event.preventDefault();
            setFilesInfo(false);
          } else if (section === "files" && Object.keys(localMarked).length > 0) {
            event.preventDefault();
            setLocalMarked({});
          }
          break;
      }
    },
    [
      activateSync,
      filesInfo,
      localMarked,
      moveFiles,
      moveHosts,
      moveKeys,
      moveSync,
      moveVault,
      providerPage,
      safeSyncCursor,
      section,
      selectedHostRow,
      switchSection,
      syncProvider,
      toggleFilesMark,
      toggleGroup,
    ],
  );

  const footerHintDesktop =
    section === "hosts"
      ? "↵ connect • ␣ group • a add • e edit • 5 files • ? help"
      : section === "keys"
        ? "g generate • i import • x export • a add to server • ? help"
        : section === "files"
          ? filesInfo
            ? "j/k next • i/esc close"
            : "tab pane • ␣ mark • i info • p chmod • ? help"
          : section === "vault"
            ? "↵ action • 4 sync • ? help"
            : syncProvider
              ? "↵ action • esc providers • r refresh • ? help"
              : "hjkl move • ↵ open • 1 hosts • 3 vault • 5 files • ? help";

  const footerHintMobile =
    section === "hosts"
      ? "tap select • 1/2/3/4/5 tabs"
      : section === "keys"
        ? "tap select • 1/2/3/4/5 tabs"
        : section === "files"
          ? filesInfo
            ? "i/esc close • 1/2/3/4/5 tabs"
            : "tap pane • i info • 1/2/3/4/5 tabs"
          : section === "vault"
            ? "tap action • 1/2/3/4/5 tabs"
            : "tap provider • 1/2/3/4/5 tabs";

  return (
    <div
      ref={containerRef}
      tabIndex={0}
      onKeyDown={onKeyDown}
      onMouseDown={() => containerRef.current?.focus()}
      className="absolute inset-0 flex flex-col overflow-hidden p-3 pt-2 font-mono text-xs leading-snug text-foreground outline-none focus-visible:ring-1 focus-visible:ring-border focus-visible:ring-inset sm:p-4 sm:pt-2 sm:text-sm md:px-5 md:pt-2 md:pb-0 md:text-[15px]"
      role="application"
      aria-label="Interactive Bast TUI demo. Use j/k or arrow keys to navigate, 1/2/3/4/5 to switch tabs."
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
            active={section === "vault"}
            onClick={() => switchSection("vault")}
          >
            [3] Vault
          </TabButton>
          <TabButton
            active={section === "sync"}
            onClick={() => switchSection("sync")}
          >
            [4] Sync
          </TabButton>
          <TabButton
            active={section === "files"}
            onClick={() => switchSection("files")}
          >
            [5] Files
          </TabButton>
        </span>
        <span className="ml-auto text-[11px] font-bold text-muted sm:text-sm md:text-[15px]">
          Demo
        </span>
      </header>

      {section === "vault" ? (
        <VaultPanel
          cursor={safeVaultCursor}
          onSelectChip={() => setVaultCursor(-1)}
          onSelectMenu={setVaultCursor}
          footerDesktop={footerHintDesktop}
          footerMobile={footerHintMobile}
        />
      ) : section === "sync" ? (
        <SyncPanel
          provider={syncProvider}
          cursor={safeSyncCursor}
          onSelectTile={(index) => {
            if (index === safeSyncCursor) {
              const tile = syncTiles[index];
              if (tile) {
                setSyncProvider(tile.id);
                setSyncCursor(0);
              }
              return;
            }
            setSyncCursor(index);
          }}
          onSelectIndex={setSyncCursor}
          onBack={() => {
            setSyncProvider("");
            setSyncCursor(0);
          }}
          footerDesktop={footerHintDesktop}
          footerMobile={footerHintMobile}
        />
      ) : section === "files" ? (
        <FilesPanel
          focus={filesFocus}
          localCursor={safeLocalCursor}
          remoteCursor={safeRemoteCursor}
          localMarked={localMarked}
          info={filesInfo}
          onFocus={setFilesFocus}
          onSelectLocal={setLocalCursor}
          onSelectRemote={setRemoteCursor}
          onToggleMark={(name) => {
            setLocalMarked((current) => {
              const next = { ...current };
              if (next[name]) {
                delete next[name];
              } else {
                next[name] = true;
              }
              return next;
            });
          }}
          onToggleInfo={() => setFilesInfo((current) => !current)}
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
  children: ReactNode;
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

function FilesPanel({
  focus,
  localCursor,
  remoteCursor,
  localMarked,
  info,
  onFocus,
  onSelectLocal,
  onSelectRemote,
  onToggleMark,
  onToggleInfo,
  footerDesktop,
  footerMobile,
}: {
  focus: 0 | 1;
  localCursor: number;
  remoteCursor: number;
  localMarked: Record<string, boolean>;
  info: boolean;
  onFocus: (pane: 0 | 1) => void;
  onSelectLocal: (index: number) => void;
  onSelectRemote: (index: number) => void;
  onToggleMark: (name: string) => void;
  onToggleInfo: () => void;
  footerDesktop: string;
  footerMobile: string;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className={`h-px shrink-0 bg-border ${bleedMargin}`} />

      <div className="flex min-h-0 flex-1 flex-col gap-2 pt-2 md:hidden">
        <FilesPane
          label="local"
          path="~/deploy"
          tone={localTone}
          focused={focus === 0}
          entries={localFiles}
          cursor={localCursor}
          marked={localMarked}
          info={info && focus === 0}
          onFocus={() => onFocus(0)}
          onSelect={onSelectLocal}
          onToggleMark={onToggleMark}
          onToggleInfo={onToggleInfo}
          compact
        />
        <div className={`h-px shrink-0 bg-border ${bleedMargin}`} />
        <FilesPane
          label="api"
          path="/var/www"
          tone={remoteTone}
          focused={focus === 1}
          entries={remoteFiles}
          cursor={remoteCursor}
          info={info && focus === 1}
          onFocus={() => onFocus(1)}
          onSelect={onSelectRemote}
          onToggleInfo={onToggleInfo}
          compact
        />
      </div>

      <div className="relative hidden min-h-0 flex-1 pt-2 md:block">
        <div className="grid h-full min-h-0 grid-cols-2">
          <FilesPane
            label="local"
            path="~/deploy"
            tone={localTone}
            focused={focus === 0}
            entries={localFiles}
            cursor={localCursor}
            marked={localMarked}
            info={info && focus === 0}
            onFocus={() => onFocus(0)}
            onSelect={onSelectLocal}
            onToggleMark={onToggleMark}
            onToggleInfo={onToggleInfo}
            className="min-h-0 overflow-hidden pr-3"
          />
          <FilesPane
            label="api"
            path="/var/www"
            tone={remoteTone}
            focused={focus === 1}
            entries={remoteFiles}
            cursor={remoteCursor}
            info={info && focus === 1}
            onFocus={() => onFocus(1)}
            onSelect={onSelectRemote}
            onToggleInfo={onToggleInfo}
            className="min-h-0 overflow-hidden pl-3 pb-10"
          />
        </div>
        <div
          aria-hidden
          className="pointer-events-none absolute top-0 bottom-0 left-1/2 z-10 w-px bg-border"
        />
        <footer className="pointer-events-none absolute inset-x-0 bottom-0 truncate pb-5 text-right text-[10px] text-muted sm:text-xs md:text-sm">
          {footerDesktop}
        </footer>
      </div>

      <footer className="mt-2 shrink-0 truncate text-right text-[10px] text-muted sm:text-xs md:hidden">
        {footerMobile}
      </footer>
    </div>
  );
}

function FilesPane({
  label,
  path,
  tone,
  focused,
  entries,
  cursor,
  marked,
  info,
  onFocus,
  onSelect,
  onToggleMark,
  onToggleInfo,
  compact = false,
  className,
}: {
  label: string;
  path: string;
  tone: string;
  focused: boolean;
  entries: FileEntry[];
  cursor: number;
  marked?: Record<string, boolean>;
  info?: boolean;
  onFocus: () => void;
  onSelect: (index: number) => void;
  onToggleMark?: (name: string) => void;
  onToggleInfo?: () => void;
  compact?: boolean;
  className?: string;
}) {
  const marker = focused ? "›" : " ";
  const entry = entries[cursor];
  return (
    <div className={className}>
      <button
        type="button"
        onClick={onFocus}
        className={`mb-1 w-full cursor-pointer truncate border-0 bg-transparent p-0 text-left font-[inherit] ${
          focused ? "font-bold" : ""
        }`}
        style={{ color: tone }}
      >
        {marker} {label}{" "}
        <span className={focused ? "" : "font-normal"}>{path}</span>
      </button>
      {info && entry ? (
        <FilesInfo entry={entry} onClose={onToggleInfo} />
      ) : (
        <ul
          role="listbox"
          aria-label={`${label} files`}
          className={compact ? "max-h-[7.5rem] overflow-hidden" : undefined}
        >
          {entries.map((item, index) => {
            const isSelected = focused && index === cursor;
            const isMarked = Boolean(marked?.[item.name]);
            const prefix = isMarked ? " •" : "  ";
            const name = item.isDir ? `${item.name}/` : item.name;
            return (
              <li key={item.name} role="option" aria-selected={isSelected}>
                <button
                  type="button"
                  onClick={() => {
                    onFocus();
                    onSelect(index);
                  }}
                  onDoubleClick={() => onToggleMark?.(item.name)}
                  className={`w-full cursor-pointer truncate whitespace-pre border-0 py-0.5 text-left font-[inherit] transition-colors ${
                    isSelected
                      ? "bg-highlight font-bold text-foreground"
                      : "bg-transparent text-foreground hover:bg-highlight/70"
                  }`}
                >
                  {prefix} {name}
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function FilesInfo({
  entry,
  onClose,
}: {
  entry: FileEntry;
  onClose?: () => void;
}) {
  const rows = [
    { label: "Name", value: entry.name },
    { label: "Type", value: entry.isDir ? "directory" : "file" },
    { label: "Size", value: entry.size },
    { label: "Mode", value: `${entry.mode}  ${entry.symbolic}` },
    { label: "Modified", value: entry.modified },
  ];
  return (
    <div>
      {rows.map((row) => (
        <p key={row.label} className="flex min-w-0 gap-3 truncate">
          <span className="inline-block w-[10ch] shrink-0 text-muted">
            {row.label}
          </span>
          <span className="min-w-0 truncate">{row.value}</span>
        </p>
      ))}
      {onClose ? (
        <button
          type="button"
          onClick={onClose}
          className="mt-2 cursor-pointer border-0 bg-transparent p-0 font-[inherit] text-muted hover:text-foreground"
        >
          i close
        </button>
      ) : null}
    </div>
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
                {row.googleCloud ? (
                  <GoogleCloudLabel />
                ) : row.label === "Amazon EC2" ? (
                  <span className="font-bold text-[#FF9900]">Amazon EC2</span>
                ) : row.label === "Microsoft Azure" ? (
                  <span className="font-bold text-[#0078D4]">Microsoft Azure</span>
                ) : (
                  row.label
                )}{" "}
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

function groupPrimaryAction(row: Extract<HostListRow, { kind: "group" }>): string | null {
  switch (row.id) {
    case "Google Cloud":
    case "Amazon EC2":
    case "Microsoft Azure":
      return "Sync now";
    default:
      return null;
  }
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
    const action = groupPrimaryAction(row);
    const hint = row.synced
      ? "␣ collapse · cloud sync (read-only)"
      : "␣ collapse · e rename";
    return (
      <>
        <p className="truncate font-bold text-accent">
          {row.googleCloud ? <GoogleCloudLabel /> : row.label}
        </p>
        {action ? (
          <div className="py-[1lh]">
            <ActionChip label={action} active />
          </div>
        ) : null}
        <p className="truncate text-muted">
          {row.count} servers · {state}
        </p>
        {action ? null : (
          <p className={`truncate text-muted ${compact ? "mt-1.5" : "mt-2"}`}>
            {hint}
          </p>
        )}
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
  const sectionGap = compact ? "mt-1.5" : "mt-2";
  return (
    <>
      <p className="min-w-0 truncate font-bold text-accent">{host.name}</p>
      <div className="py-[1lh]">
        <ActionChip label="Connect" active />
      </div>
      <p className="truncate">{host.destination}</p>
      <p className="truncate text-muted">{host.summary}</p>
      {host.access && host.access.length > 0 ? (
        <div className={`space-y-0.5 ${sectionGap}`}>
          <p className="text-muted">Access</p>
          {host.access.map((row) => (
            <DetailLine key={row.label} {...row} compact={compact} />
          ))}
        </div>
      ) : null}
      {host.about && host.about.length > 0 ? (
        <div className={`space-y-0.5 ${sectionGap}`}>
          <p className="text-muted">About</p>
          {host.about.map((row) => (
            <DetailLine key={row.label} {...row} compact={compact} />
          ))}
        </div>
      ) : null}
      {host.promote ? (
        <p className={`font-bold text-accent ${sectionGap}`}>
          [p] Promote to Bast managed
        </p>
      ) : null}
    </>
  );
}

function ActionChip({
  label,
  active = false,
  onClick,
}: {
  label: string;
  active?: boolean;
  onClick?: () => void;
}) {
  const className = `w-fit border-0 px-1.5 py-0.5 text-[10px] font-bold tracking-wide sm:text-xs ${
    active ? "bg-accent text-white" : "bg-transparent text-foreground"
  } ${onClick ? "cursor-pointer" : ""}`;
  const text = ` ${label} `;
  if (!onClick) {
    return <span className={className}>{text}</span>;
  }
  return (
    <button type="button" onClick={onClick} className={className}>
      {text}
    </button>
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
          [a] Add to server
        </p>
        {keyItem.rows.map((row) => (
          <DetailLine key={row.label} {...row} compact={compact} />
        ))}
      </div>
    </>
  );
}

function DemoPane({
  children,
  footerDesktop,
  footerMobile,
}: {
  children: ReactNode;
  footerDesktop: string;
  footerMobile: string;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className={`h-px shrink-0 bg-border ${bleedMargin}`} />
      <div className="min-h-0 flex-1 overflow-hidden pt-1.5">{children}</div>
      <footer className="mt-2 hidden shrink-0 truncate text-right text-[10px] text-muted sm:text-xs md:block md:text-sm">
        {footerDesktop}
      </footer>
      <footer className="mt-2 shrink-0 truncate text-right text-[10px] text-muted sm:text-xs md:hidden">
        {footerMobile}
      </footer>
    </div>
  );
}

function SyncPanel({
  provider,
  cursor,
  onSelectTile,
  onSelectIndex,
  onBack,
  footerDesktop,
  footerMobile,
}: {
  provider: SyncProvider;
  cursor: number;
  onSelectTile: (index: number) => void;
  onSelectIndex: (index: number) => void;
  onBack: () => void;
  footerDesktop: string;
  footerMobile: string;
}) {
  const page = provider ? providerPages[provider] : null;
  const selectedTile = syncTiles[cursor];
  return (
    <DemoPane footerDesktop={footerDesktop} footerMobile={footerMobile}>
      {page && provider ? (
        <ProviderPage
          provider={provider}
          page={page}
          cursor={cursor}
          onSelect={onSelectIndex}
          onBack={onBack}
        />
      ) : (
        <>
          <div className="grid min-w-0 grid-cols-1 gap-x-2 gap-y-1 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            {syncTiles.map((tile, index) => (
              <SyncTileButton
                key={tile.id}
                tile={tile}
                selected={index === cursor}
                onClick={() => onSelectTile(index)}
              />
            ))}
          </div>
          {selectedTile ? (
            <p className="mt-2 truncate pl-2 text-muted">
              {selectedTile.description}
            </p>
          ) : null}
        </>
      )}
    </DemoPane>
  );
}

function BoxRule({
  start,
  end,
  className,
}: {
  start: string;
  end: string;
  className: string;
}) {
  return (
    <span className={`flex min-w-0 ${className}`} aria-hidden>
      <span className="shrink-0">{start}</span>
      <span className="min-w-0 flex-1 overflow-hidden whitespace-nowrap">
        {"─".repeat(64)}
      </span>
      <span className="shrink-0">{end}</span>
    </span>
  );
}

function SyncTileButton({
  tile,
  selected,
  onClick,
}: {
  tile: SyncTile;
  selected: boolean;
  onClick: () => void;
}) {
  const edge = selected ? "text-accent" : "text-muted";
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full min-w-0 cursor-pointer flex-col border-0 bg-transparent p-0 text-left font-[inherit]"
    >
      <BoxRule start="┌" end="┐" className={edge} />
      <span className="flex min-w-0">
        <span className={`shrink-0 ${edge}`}>│</span>
        <span className="min-w-0 flex-1 truncate px-1 font-bold">
          <ProviderTitle provider={tile.id} />
        </span>
        <span className={`shrink-0 ${edge}`}>│</span>
      </span>
      <span className="flex min-w-0">
        <span className={`shrink-0 ${edge}`}>│</span>
        <span className="min-w-0 flex-1 truncate px-1 text-muted">
          {tile.detail}
        </span>
        <span className={`shrink-0 ${edge}`}>│</span>
      </span>
      <BoxRule start="└" end="┘" className={edge} />
    </button>
  );
}

function ProviderPage({
  provider,
  page,
  cursor,
  onSelect,
  onBack,
}: {
  provider: Exclude<SyncProvider, "">;
  page: ProviderPage;
  cursor: number;
  onSelect: (index: number) => void;
  onBack: () => void;
}) {
  const chipCount = page.chips.length;
  return (
    <>
      <div className="mb-1 flex items-center justify-between gap-2">
        <p className="truncate font-bold">
          <ProviderTitle provider={provider} />
        </p>
        <button
          type="button"
          onClick={onBack}
          className="cursor-pointer border-0 bg-transparent p-0 font-[inherit] text-muted hover:text-foreground"
        >
          esc
        </button>
      </div>
      <div className="mb-1 space-y-0.5">
        {page.identity.map((line) => (
          <IdentityLine key={line} line={line} />
        ))}
      </div>
      <div className="flex flex-wrap gap-x-4 py-[1lh]">
        {page.chips.map((label, index) => (
          <ActionChip
            key={label}
            label={label}
            active={cursor === index}
            onClick={() => onSelect(index)}
          />
        ))}
      </div>
      <ul role="listbox" aria-label="sync config" className="max-w-md">
        {page.config.map((item, index) => {
          const itemIndex = chipCount + index;
          return (
            <MenuRow
              key={item.label}
              item={item}
              selected={cursor === itemIndex}
              onSelect={() => onSelect(itemIndex)}
            />
          );
        })}
      </ul>
    </>
  );
}

function VaultPanel({
  cursor,
  onSelectChip,
  onSelectMenu,
  footerDesktop,
  footerMobile,
}: {
  cursor: number;
  onSelectChip: () => void;
  onSelectMenu: (index: number) => void;
  footerDesktop: string;
  footerMobile: string;
}) {
  return (
    <DemoPane footerDesktop={footerDesktop} footerMobile={footerMobile}>
      <p className="font-bold text-accent">Vault</p>
      <p className="truncate">you@example.com</p>
      <p className="truncate">
        <span className="text-[#10B981]">unlocked</span>
        <span className="text-muted"> · 13:55:02 · a1b2c3d4</span>
      </p>
      <div className="py-[1lh]">
        <ActionChip
          label="Sync"
          active={cursor < 0}
          onClick={onSelectChip}
        />
      </div>
      <ul role="listbox" aria-label="vault" className="max-w-md">
        {vaultMenu.map((item, index) => (
          <MenuRow
            key={item.label}
            item={item}
            selected={cursor === index}
            onSelect={() => onSelectMenu(index)}
          />
        ))}
      </ul>
    </DemoPane>
  );
}

function MenuRow({
  item,
  selected,
  onSelect,
}: {
  item: SyncMenuItem;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <li role="option" aria-selected={selected}>
      <button
        type="button"
        disabled={item.disabled}
        onClick={onSelect}
        className={`flex w-full items-baseline gap-4 border-0 py-0.5 text-left font-[inherit] transition-colors ${
          item.disabled
            ? "cursor-default bg-transparent text-muted"
            : selected
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
      {selected && item.description ? (
        <p className="pl-4 text-muted">{item.description}</p>
      ) : null}
    </li>
  );
}

function ProviderTitle({
  provider,
}: {
  provider: Exclude<SyncProvider, "">;
}) {
  switch (provider) {
    case "gcp":
      return <GoogleCloudLabel />;
    case "aws":
      return <span className="font-bold text-[#FF9900]">Amazon EC2</span>;
    case "azure":
      return <span className="font-bold text-[#0078D4]">Microsoft Azure</span>;
    case "box":
      return <span className="font-bold text-foreground">Box</span>;
    case "upstash":
      return <span className="font-bold text-[#00E9A3]">Upstash</span>;
  }
}

function IdentityLine({ line }: { line: string }) {
  if (line.startsWith("enabled")) {
    return (
      <p className="truncate">
        <span className="text-[#10B981]">enabled</span>
        <span className="text-muted">{line.slice("enabled".length)}</span>
      </p>
    );
  }
  return <p className="truncate text-muted">{line}</p>;
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
