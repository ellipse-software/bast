"use client";

import localFont from "next/font/local";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

const nerdSymbols = localFont({
  src: "../fonts/bast-nerd-symbols.woff2",
  display: "swap",
  preload: false,
  adjustFontFallback: false,
});

type Section = "hosts" | "keys" | "vault" | "sync" | "files";

export type DemoSection = Section;
type SyncProvider =
  | "gcp"
  | "aws"
  | "azure"
  | "boat"
  | "upstash"
  | "vercel"
  | "hetzner"
  | "bitqate"
  | "railway"
  | "fly"
  | "digitalocean"
  | "boxd";

const providerNerdIcon: Record<Exclude<SyncProvider, "">, string> = {
  gcp: "\ue7f1",
  aws: "\ue7ad",
  azure: "\ue754",
  boat: "\u{f0ec8}",
  upstash: "\uf1b2",
  vercel: "\u25b2",
  hetzner: "\uf233",
  bitqate: "\uf0a0",
  railway: "\uf238",
  fly: "\uf072",
  digitalocean: "\ue7ae",
  boxd: "\u{f01a7}",
};

const providerLabel: Record<Exclude<SyncProvider, "">, string> = {
  gcp: "Google Cloud",
  aws: "Amazon EC2",
  azure: "Microsoft Azure",
  boat: "Boat",
  upstash: "Upstash",
  vercel: "Vercel",
  hetzner: "Hetzner Cloud",
  bitqate: "BitQate",
  railway: "Railway",
  fly: "Fly.io",
  digitalocean: "DigitalOcean",
  boxd: "boxd",
};

const providerColor: Record<Exclude<SyncProvider, "">, string> = {
  gcp: "text-[#4285F4]",
  aws: "text-[#FF9900]",
  azure: "text-[#0078D4]",
  boat: "text-[#263FA9]",
  upstash: "text-[#00E9A3]",
  vercel: "text-foreground",
  hetzner: "text-[#D50C2D]",
  bitqate: "text-foreground",
  railway: "text-[#853BCC]",
  fly: "text-[#8B5CF6]",
  digitalocean: "text-[#0080FF]",
  boxd: "text-[#F4F4F5]",
};

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
        access: [
          { label: "Auth", value: "~/.ssh/bast/keys/work" },
          { label: "Jump", value: "bastion" },
        ],
        about: [
          { label: "Env", value: "production" },
          { label: "Tags", value: "web, api, edge" },
          { label: "Used", value: "2026-07-26 14:02 (128)" },
          { label: "Notes", value: "public API behind the edge proxy" },
        ],
      },
      {
        name: "bastion",
        destination: "ops@bastion.prod.example.com",
        summary: "Bast managed · known host",
        access: [{ label: "Auth", value: "~/.ssh/bast/keys/work" }],
        about: [
          { label: "Env", value: "production" },
          { label: "Tags", value: "jump" },
          { label: "Used", value: "2026-07-26 13:11 (46)" },
          { label: "Notes", value: "jump host for the prod VPC" },
        ],
      },
      {
        name: "metrics",
        destination: "obs@metrics.prod.example.com",
        summary: "Bast managed · known host",
        access: [
          { label: "Auth", value: "~/.ssh/bast/keys/work" },
          { label: "Jump", value: "bastion" },
        ],
        about: [
          { label: "Env", value: "production" },
          { label: "Tags", value: "obs" },
          { label: "Used", value: "2026-07-25 09:18 (7)" },
          { label: "Notes", value: "Prometheus and Grafana" },
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
            access: [
              { label: "Auth", value: "~/.ssh/bast/keys/work" },
              { label: "SSH name", value: "gcp_demo_web-01" },
            ],
            about: [
              { label: "Env", value: "demo" },
              { label: "Tags", value: "web, public" },
              { label: "Used", value: "2026-07-26 13:40 (24)" },
              { label: "Notes", value: "us-central1-a · e2-medium" },
            ],
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
            about: [
              { label: "Env", value: "demo" },
              { label: "Tags", value: "gpu, batch" },
              { label: "Used", value: "2026-07-24 02:16 (3)" },
              { label: "Notes", value: "a2-highgpu-1g · IAP only" },
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
              { label: "Auth", value: "~/.ssh/bast/keys/work" },
              { label: "Jump", value: "EC2 Instance Connect Endpoint" },
              {
                label: "SSH name",
                value: "aws_production_us-east-1_i-0123456789abcdef0",
              },
            ],
            about: [
              { label: "Env", value: "production" },
              { label: "Tags", value: "worker, queue" },
              { label: "Used", value: "2026-07-27 18:04 (11)" },
              { label: "Notes", value: "i-0123456789abcdef0 · us-east-1a" },
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
              { label: "Auth", value: "~/.ssh/bast/keys/work" },
              { label: "Jump", value: "Azure Bastion tunnel" },
              { label: "SSH name", value: "azure_Production_apps_api-01" },
            ],
            about: [
              { label: "Env", value: "production" },
              { label: "Tags", value: "api" },
              { label: "Used", value: "2026-07-28 10:02 (6)" },
              { label: "Notes", value: "Standard_D2s_v5 · eastus" },
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
    about: [
      { label: "Tags", value: "git" },
      { label: "Used", value: "2026-07-26 16:44 (53)" },
    ],
    promote: true,
  },
];

const keys: KeyItem[] = [
  {
    name: "work",
    inAgent: true,
    summary: "ed25519 · Bast managed · agent cached",
    fingerprint: "SHA256:n4Q8mP2sL6vR1tY9wK3xC7bH5dF0gJ8aE2uI4oZ6qW",
    rows: [
      { label: "Private", value: "~/.ssh/bast/keys/work" },
      { label: "Public", value: "~/.ssh/bast/keys/work.pub" },
      { label: "Used by", value: "api, bastion, metrics, web-01" },
    ],
  },
  {
    name: "id_rsa_legacy",
    summary: "rsa · external",
    fingerprint: "SHA256:a1B2c3D4e5F6g7H8i9J0kLmNoPqRsTuVwXyZ0123456",
    rows: [
      { label: "Private", value: "~/.ssh/id_rsa" },
      { label: "Public", value: "~/.ssh/id_rsa.pub" },
    ],
  },
  {
    name: "github_deploy",
    summary: "ed25519 · external · agent cached",
    inAgent: true,
    fingerprint: "SHA256:z9Y8x7W6v5U4t3S2r1Q0pOnMlKjIhGfEdCbA9876543",
    rows: [
      { label: "Private", value: "~/.ssh/id_ed25519_github" },
      { label: "Used by", value: "github.com" },
    ],
  },
];

type SyncSetting = {
  label: string;
  value: string;
};

type SyncProviderModel = {
  id: SyncProvider;
  status: "on" | "off" | "error" | "syncing";
  count?: number;
  error?: string;
  settings: SyncSetting[];
  lifecycle: SyncSetting[];
};

const syncProviders: SyncProviderModel[] = [
  {
    id: "gcp",
    status: "on",
    count: 2,
    settings: [
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "on" },
      { label: "SSH user", value: "ubuntu" },
      { label: "Projects", value: "all" },
      { label: "Add service account", value: "" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [{ label: "Sync now", value: "" }],
  },
  {
    id: "aws",
    status: "on",
    count: 4,
    settings: [
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "off" },
      { label: "SSH user", value: "ec2-user" },
      { label: "Profiles", value: "all" },
      { label: "Regions", value: "all" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [{ label: "Sync now", value: "" }],
  },
  {
    id: "azure",
    status: "error",
    error:
      "could not find Azure CLI; install Azure CLI 2.62 or later and run az login",
    settings: [
      { label: "Login", value: "" },
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "off" },
      { label: "SSH user", value: "azureuser" },
      { label: "Subscriptions", value: "all" },
      { label: "Resource groups", value: "all" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [{ label: "Sync now", value: "" }],
  },
  {
    id: "boat",
    status: "on",
    count: 2,
    settings: [
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "on" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [
      { label: "Sync now", value: "" },
      { label: "New sandbox", value: "" },
    ],
  },
  {
    id: "upstash",
    status: "on",
    count: 2,
    settings: [
      { label: "New token", value: "" },
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "on" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [
      { label: "Sync now", value: "" },
      { label: "New box", value: "" },
    ],
  },
  {
    id: "vercel",
    status: "on",
    count: 1,
    settings: [
      { label: "New token", value: "" },
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "on" },
      { label: "Team", value: "on" },
      { label: "Projects", value: "on" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [
      { label: "Sync now", value: "" },
      { label: "Cleanup", value: "2 unrestorable" },
      { label: "New sandbox", value: "" },
    ],
  },
  {
    id: "hetzner",
    status: "on",
    count: 1,
    settings: [
      { label: "New token", value: "" },
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "off" },
      { label: "Private IP", value: "off" },
      { label: "SSH user", value: "root" },
      { label: "SSH port", value: "22" },
      { label: "Contexts", value: "all" },
      { label: "Locations", value: "all" },
      { label: "Remove token", value: "" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [{ label: "Sync now", value: "" }],
  },
  {
    id: "bitqate",
    status: "on",
    count: 2,
    settings: [
      { label: "New token", value: "" },
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "on" },
      { label: "SSH user", value: "root" },
      { label: "SSH port", value: "22" },
      { label: "Regions", value: "all" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [{ label: "Sync now", value: "" }],
  },
  {
    id: "railway",
    status: "on",
    count: 2,
    settings: [
      { label: "New token", value: "" },
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "on" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [
      { label: "Sync now", value: "" },
      { label: "New service", value: "" },
    ],
  },
  {
    id: "fly",
    status: "off",
    settings: [
      { label: "Login", value: "" },
      { label: "Enabled", value: "off" },
      { label: "Auto-sync", value: "off" },
      { label: "SSH user", value: "root" },
      { label: "Orgs", value: "all" },
      { label: "Apps", value: "all" },
    ],
    lifecycle: [{ label: "Sync now", value: "" }],
  },
  {
    id: "digitalocean",
    status: "on",
    count: 3,
    settings: [
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "on" },
      { label: "SSH user", value: "root" },
      { label: "Contexts", value: "all" },
      { label: "Regions", value: "all" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [
      { label: "Sync now", value: "" },
      { label: "New droplet", value: "" },
    ],
  },
  {
    id: "boxd",
    status: "on",
    count: 2,
    settings: [
      { label: "Enabled", value: "on" },
      { label: "Auto-sync", value: "on" },
      { label: "Orgs", value: "all" },
      { label: "Disconnect", value: "" },
    ],
    lifecycle: [
      { label: "Sync now", value: "" },
      { label: "New computer", value: "" },
    ],
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

const filePreviewBodies: Record<string, string> = {
  "notes.md":
    "# roll\n\napi first\njump box last\nkeep secrets out of git\ncheck health after deploy\nrevert on 5xx\nwrite the postmortem",
  "secret.env": "REGION=us-east-1\nLOG_LEVEL=info\nKEEPALIVE=30",
  "id_ed25519.pub": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeDemoKeyOnly work",
  README:
    "api\n\nEdge proxy in front.\nJump through bastion.\nKey is work.\nHealth: /ready",
  "docker-compose.yml":
    "services:\n  api:\n    image: registry.example.com/api:2026.07.26\n    restart: unless-stopped\n    ports:\n      - \"443:443\"",
};

const previewVisibleLines = 12;

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

type FilePane =
  | { mode: "picker"; cursor: number }
  | { mode: "local"; cursor: number }
  | { mode: "remote"; cursor: number; hostId: string };

function clampIndex(index: number, count: number): number {
  if (count <= 0) return 0;
  return Math.max(0, Math.min(count - 1, index));
}

function filePaneCount(pane: FilePane, hostCount: number): number {
  if (pane.mode === "picker") return 1 + hostCount;
  if (pane.mode === "local") return localFiles.length;
  return remoteFiles.length;
}

function browserEntries(pane: FilePane): FileEntry[] | null {
  if (pane.mode === "local") return localFiles;
  if (pane.mode === "remote") return remoteFiles;
  return null;
}

function hostById(rows: HostListRow[], id: string): HostItem | undefined {
  for (const row of rows) {
    if (row.kind === "host" && row.id === id) return row.host;
  }
  return undefined;
}

function syncDetailRows(provider: SyncProviderModel): Array<SyncSetting & { section: "Settings" | "Lifecycle" }> {
  return [
    ...provider.settings.map((row) => ({ section: "Settings" as const, ...row })),
    ...provider.lifecycle.map((row) => ({ section: "Lifecycle" as const, ...row })),
  ];
}

function syncValueClass(value: string): string {
  switch (value) {
    case "on":
      return "text-[#10B981]";
    case "off":
    case "syncing":
    case "all":
      return "text-muted";
    case "error":
      return "text-[#EF4444]";
    default:
      return "text-foreground";
  }
}

function scrollSelectedInto(container: HTMLElement | null) {
  const item = container?.querySelector<HTMLElement>('[aria-selected="true"]');
  if (!container || !item) return;
  const listRect = container.getBoundingClientRect();
  const itemRect = item.getBoundingClientRect();
  if (itemRect.top < listRect.top) {
    container.scrollTop -= listRect.top - itemRect.top;
  } else if (itemRect.bottom > listRect.bottom) {
    container.scrollTop += itemRect.bottom - listRect.bottom;
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

export function TuiDemo({
  showLabel = true,
  showHints = true,
  roomy = false,
  initialSection = "hosts",
}: {
  showLabel?: boolean;
  showHints?: boolean;
  roomy?: boolean;
  initialSection?: Section;
}) {
  const [section, setSection] = useState<Section>(initialSection);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const [hostCursor, setHostCursor] = useState(() =>
    defaultHostCursor(buildHostRows({})),
  );
  const [keyCursor, setKeyCursor] = useState(0);
  const [syncListIndex, setSyncListIndex] = useState(0);
  const [syncDetail, setSyncDetail] = useState(false);
  const [syncDetailCursor, setSyncDetailCursor] = useState(0);
  const [vaultCursor, setVaultCursor] = useState(-1);
  const [filesFocus, setFilesFocus] = useState<0 | 1>(0);
  const [filePanes, setFilePanes] = useState<[FilePane, FilePane]>([
    { mode: "picker", cursor: 0 },
    { mode: "local", cursor: 2 },
  ]);
  const [fileMarks, setFileMarks] = useState<
    [Record<string, boolean>, Record<string, boolean>]
  >([{}, { "secret.env": true }]);
  const [filesInfo, setFilesInfo] = useState(false);
  const [filesPreview, setFilesPreview] = useState(false);
  const [previewOffset, setPreviewOffset] = useState(0);
  const containerRef = useRef<HTMLDivElement>(null);

  const hostRows = useMemo(() => buildHostRows(collapsed), [collapsed]);
  const safeHostCursor = Math.min(hostCursor, Math.max(0, hostRows.length - 1));
  const selectedHostRow = hostRows[safeHostCursor];
  const selectedKey = keys[Math.min(keyCursor, keys.length - 1)];

  const safeSyncListIndex = clampIndex(syncListIndex, syncProviders.length);
  const syncProvider = syncProviders[safeSyncListIndex];
  const syncRows = syncProvider ? syncDetailRows(syncProvider) : [];
  const safeSyncDetailCursor = clampIndex(syncDetailCursor, syncRows.length);
  const safeVaultCursor = Math.min(vaultCursor, vaultMenu.length - 1);
  const focusedPane = filePanes[filesFocus];
  const focusedFileEntry = (() => {
    const entries = browserEntries(focusedPane);
    if (!entries) return undefined;
    return entries[clampIndex(focusedPane.cursor, entries.length)];
  })();

  const switchSection = useCallback((next: Section) => {
    setSection(next);
    setFilesPreview(false);
    setPreviewOffset(0);
    if (next === "sync") {
      setSyncListIndex(0);
      setSyncDetail(false);
      setSyncDetailCursor(0);
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
      if (syncDetail) {
        setSyncDetailCursor((current) =>
          clampIndex(current + delta, syncRows.length),
        );
        return;
      }
      setSyncListIndex((current) =>
        clampIndex(current + delta, syncProviders.length),
      );
      setSyncDetailCursor(0);
    },
    [syncDetail, syncRows.length],
  );

  const openSyncDetail = useCallback(() => {
    setSyncDetail(true);
    setSyncDetailCursor(0);
  }, []);

  const moveVault = useCallback((delta: number) => {
    setVaultCursor((current) =>
      Math.max(-1, Math.min(vaultMenu.length - 1, current + delta)),
    );
  }, []);

  const moveFiles = useCallback(
    (delta: number) => {
      setFilePanes((current) => {
        const next: [FilePane, FilePane] = [current[0], current[1]];
        const pane = next[filesFocus];
        next[filesFocus] = {
          ...pane,
          cursor: clampIndex(
            pane.cursor + delta,
            filePaneCount(pane, hostRows.length),
          ),
        };
        return next;
      });
    },
    [filesFocus, hostRows.length],
  );

  const setFilePane = useCallback((index: 0 | 1, pane: FilePane) => {
    setFilePanes((current) => {
      const next: [FilePane, FilePane] = [current[0], current[1]];
      next[index] = pane;
      return next;
    });
  }, []);

  const toggleFilesPreview = useCallback(() => {
    const entry = focusedFileEntry;
    if (!entry || entry.isDir) return;
    setFilesInfo(false);
    setFilesPreview((open) => {
      if (!open) {
        setPreviewOffset(0);
      }
      return !open;
    });
  }, [focusedFileEntry]);

  const scrollFilesPreview = useCallback(
    (delta: number) => {
      const entry = focusedFileEntry;
      const body = entry ? (filePreviewBodies[entry.name] ?? "") : "";
      const lineCount = body === "" ? 0 : body.split("\n").length;
      const maxOffset = Math.max(0, lineCount - previewVisibleLines);
      setPreviewOffset((current) =>
        Math.max(0, Math.min(maxOffset, current + delta)),
      );
    },
    [focusedFileEntry],
  );

  const stepFilesPreview = useCallback(
    (delta: number) => {
      const entries = browserEntries(focusedPane);
      if (!entries) return;
      const cursor = clampIndex(focusedPane.cursor, entries.length);
      for (let i = 1; i < entries.length; i += 1) {
        const idx = cursor + delta * i;
        if (idx < 0 || idx >= entries.length) return;
        if (entries[idx].isDir) continue;
        setFilePane(filesFocus, { ...focusedPane, cursor: idx });
        setPreviewOffset(0);
        setFilesInfo(false);
        setFilesPreview(true);
        return;
      }
    },
    [filesFocus, focusedPane, setFilePane],
  );

  const toggleFilesMark = useCallback(() => {
    if (focusedPane.mode !== "local") return;
    const entries = browserEntries(focusedPane);
    const entry = entries?.[clampIndex(focusedPane.cursor, entries.length)];
    if (!entry) return;
    setFileMarks((current) => {
      const next: [Record<string, boolean>, Record<string, boolean>] = [
        { ...current[0] },
        { ...current[1] },
      ];
      const marks = next[filesFocus];
      if (marks[entry.name]) delete marks[entry.name];
      else marks[entry.name] = true;
      return next;
    });
  }, [filesFocus, focusedPane]);

  const openFilesPickerRow = useCallback(
    (index: 0 | 1, cursor: number) => {
      setFilesFocus(index);
      if (cursor <= 0) {
        setFilePane(index, { mode: "local", cursor: 0 });
        return;
      }
      const row = hostRows[cursor - 1];
      if (!row) return;
      if (row.kind === "group") {
        toggleGroup(row.id);
        setFilePane(index, { mode: "picker", cursor });
        return;
      }
      setFilePane(index, { mode: "remote", cursor: 0, hostId: row.id });
    },
    [hostRows, setFilePane, toggleGroup],
  );

  const activateFiles = useCallback(() => {
    const pane = filePanes[filesFocus];
    if (pane.mode === "picker") {
      openFilesPickerRow(
        filesFocus,
        clampIndex(pane.cursor, filePaneCount(pane, hostRows.length)),
      );
      return;
    }
    toggleFilesPreview();
  }, [filePanes, filesFocus, hostRows.length, openFilesPickerRow, toggleFilesPreview]);

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
          else if (section === "files" && filesPreview) scrollFilesPreview(1);
          else if (section === "files") moveFiles(1);
          else if (section === "vault") moveVault(1);
          else moveSync(1);
          break;
        case "k":
        case "ArrowUp":
          event.preventDefault();
          if (section === "hosts") moveHosts(-1);
          else if (section === "keys") moveKeys(-1);
          else if (section === "files" && filesPreview) scrollFilesPreview(-1);
          else if (section === "files") moveFiles(-1);
          else if (section === "vault") moveVault(-1);
          else moveSync(-1);
          break;
        case "[":
          if (section === "files" && filesPreview) {
            event.preventDefault();
            stepFilesPreview(-1);
          }
          break;
        case "]":
          if (section === "files" && filesPreview) {
            event.preventDefault();
            stepFilesPreview(1);
          }
          break;
        case "h":
        case "ArrowLeft":
          if (section === "sync" && syncDetail) {
            event.preventDefault();
            setSyncDetail(false);
          }
          break;
        case "l":
        case "ArrowRight":
          if (section === "sync" && !syncDetail) {
            event.preventDefault();
            openSyncDetail();
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
          } else if (section === "sync") {
            event.preventDefault();
            setSyncDetail((current) => !current);
          }
          break;
        case "i":
          if (section === "files" && focusedPane.mode !== "picker") {
            event.preventDefault();
            setFilesPreview(false);
            setPreviewOffset(0);
            setFilesInfo((current) => !current);
          }
          break;
        case "o":
          if (section === "files" && focusedPane.mode !== "picker") {
            event.preventDefault();
            toggleFilesPreview();
          }
          break;
        case "Enter":
          event.preventDefault();
          if (section === "hosts" && selectedHostRow?.kind === "group") {
            toggleGroup(selectedHostRow.id);
          } else if (section === "sync") {
            if (!syncDetail) openSyncDetail();
          } else if (section === "files") {
            activateFiles();
          }
          break;
        case " ":
          event.preventDefault();
          if (section === "hosts" && selectedHostRow?.kind === "group") {
            toggleGroup(selectedHostRow.id);
          } else if (section === "sync") {
            if (!syncDetail) openSyncDetail();
          } else if (section === "files" && !filesPreview) {
            if (focusedPane.mode === "picker") {
              const cursor = clampIndex(
                focusedPane.cursor,
                filePaneCount(focusedPane, hostRows.length),
              );
              const row = cursor === 0 ? undefined : hostRows[cursor - 1];
              if (row?.kind === "group") toggleGroup(row.id);
            } else {
              toggleFilesMark();
            }
          }
          break;
        case "Escape":
          if (section === "sync" && syncDetail) {
            event.preventDefault();
            setSyncDetail(false);
          } else if (section === "files" && filesPreview) {
            event.preventDefault();
            setFilesPreview(false);
            setPreviewOffset(0);
          } else if (section === "files" && filesInfo) {
            event.preventDefault();
            setFilesInfo(false);
          } else if (
            section === "files" &&
            focusedPane.mode === "local" &&
            Object.keys(fileMarks[filesFocus]).length > 0
          ) {
            event.preventDefault();
            setFileMarks((current) => {
              const next: [Record<string, boolean>, Record<string, boolean>] = [
                current[0],
                current[1],
              ];
              next[filesFocus] = {};
              return next;
            });
          } else if (section === "files" && focusedPane.mode === "remote") {
            event.preventDefault();
            setFilePane(filesFocus, { mode: "picker", cursor: 0 });
          } else if (section === "files") {
            event.preventDefault();
            switchSection("hosts");
          }
          break;
      }
    },
    [
      activateFiles,
      fileMarks,
      filesFocus,
      filesInfo,
      filesPreview,
      focusedPane,
      hostRows,
      moveFiles,
      scrollFilesPreview,
      stepFilesPreview,
      moveHosts,
      moveKeys,
      moveSync,
      moveVault,
      openSyncDetail,
      section,
      selectedHostRow,
      setFilePane,
      switchSection,
      syncDetail,
      toggleFilesMark,
      toggleFilesPreview,
      toggleGroup,
    ],
  );

  const footerHintDesktop = showHints ? "/ for shortcuts" : "";
  const footerHintMobile = showHints ? "/ for shortcuts" : "";

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
            roomy={roomy}
            onClick={() => switchSection("hosts")}
          >
            [1] Hosts
          </TabButton>
          <TabButton
            active={section === "keys"}
            roomy={roomy}
            onClick={() => switchSection("keys")}
          >
            [2] Keys
          </TabButton>
          <TabButton
            active={section === "vault"}
            roomy={roomy}
            onClick={() => switchSection("vault")}
          >
            [3] Vault
          </TabButton>
          <TabButton
            active={section === "sync"}
            roomy={roomy}
            onClick={() => switchSection("sync")}
          >
            [4] Sync
          </TabButton>
          <TabButton
            active={section === "files"}
            roomy={roomy}
            onClick={() => switchSection("files")}
          >
            [5] Files
          </TabButton>
        </span>
        {showLabel ? (
          <span className="ml-auto text-[11px] font-bold text-muted sm:text-sm md:text-[15px]">
            Demo
          </span>
        ) : null}
      </header>

      {section === "vault" ? (
        <VaultPanel
          cursor={safeVaultCursor}
          onSelectChip={() => setVaultCursor(-1)}
          onSelectMenu={setVaultCursor}
          footerDesktop={footerHintDesktop}
          footerMobile={footerHintMobile}
          roomy={roomy}
        />
      ) : section === "sync" ? (
        <SyncPanel
          listIndex={safeSyncListIndex}
          detail={syncDetail}
          detailCursor={safeSyncDetailCursor}
          onSelectProvider={(index) => {
            setSyncListIndex(index);
            setSyncDetail(true);
            setSyncDetailCursor(0);
          }}
          onSelectSetting={(index) => {
            setSyncDetail(true);
            setSyncDetailCursor(index);
          }}
          footerDesktop={footerHintDesktop}
          footerMobile={footerHintMobile}
          roomy={roomy}
        />
      ) : section === "files" ? (
        <FilesPanel
          panes={filePanes}
          focus={filesFocus}
          hostRows={hostRows}
          collapsed={collapsed}
          marks={fileMarks}
          roomy={roomy}
          info={filesInfo}
          preview={filesPreview}
          previewOffset={previewOffset}
          onFocus={setFilesFocus}
          onOpenPickerRow={openFilesPickerRow}
          onSelectBrowser={(index, cursor) => {
            const pane = filePanes[index];
            if (pane.mode === "picker") return;
            setFilesFocus(index);
            setFilePane(index, { ...pane, cursor });
          }}
          onToggleMark={(index, name) => {
            setFilesFocus(index);
            setFileMarks((current) => {
              const next: [Record<string, boolean>, Record<string, boolean>] = [
                { ...current[0] },
                { ...current[1] },
              ];
              if (next[index][name]) delete next[index][name];
              else next[index][name] = true;
              return next;
            });
          }}
          onToggleInfo={() => {
            setFilesPreview(false);
            setFilesInfo((current) => !current);
          }}
          onTogglePreview={toggleFilesPreview}
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
                roomy={roomy}
                onSelect={setHostCursor}
                onToggleGroup={toggleGroup}
                className="shrink-0 pt-1.5"
              />
            ) : (
              <KeyList
                items={keys}
                cursor={keyCursor}
                roomy={roomy}
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
                  roomy={roomy}
                  onSelect={setHostCursor}
                  onToggleGroup={toggleGroup}
                  className={`min-h-0 pt-1.5 ${roomy ? "overflow-y-auto" : "overflow-hidden"}`}
                />
              ) : (
                <KeyList
                  items={keys}
                  cursor={keyCursor}
                  roomy={roomy}
                  onSelect={setKeyCursor}
                  className={`min-h-0 pt-1.5 ${roomy ? "overflow-y-auto" : "overflow-hidden"}`}
                />
              )}

              <div
                className={`min-w-0 overflow-hidden pt-1.5 pl-3 md:pl-4 ${
                  footerHintDesktop ? "pb-10" : "pb-3"
                }`}
              >
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

            {footerHintDesktop ? (
              <footer className="pointer-events-none absolute bottom-0 left-0 right-[var(--demo-gutter,0px)] pb-5 pl-3 text-left text-[10px] text-muted sm:text-xs md:text-sm">
                {footerHintDesktop}
              </footer>
            ) : null}
          </div>

          {footerHintMobile ? (
            <footer className="mt-2 shrink-0 text-left text-[10px] text-muted sm:text-xs md:hidden md:text-sm">
              {footerHintMobile}
            </footer>
          ) : null}
        </>
      )}
    </div>
  );
}

function TabButton({
  active,
  roomy = false,
  onClick,
  children,
}: {
  active: boolean;
  roomy?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`cursor-pointer border-0 bg-transparent font-[inherit] font-bold transition-colors ${
        roomy ? "px-1.5 py-1" : "p-0"
      } ${active ? "text-accent" : "text-muted hover:text-foreground"}`}
    >
      {children}
    </button>
  );
}

function FilesPanel({
  panes,
  focus,
  hostRows,
  collapsed,
  marks,
  info,
  preview,
  previewOffset,
  onFocus,
  onOpenPickerRow,
  onSelectBrowser,
  onToggleMark,
  onToggleInfo,
  onTogglePreview,
  roomy = false,
  footerDesktop,
  footerMobile,
}: {
  panes: [FilePane, FilePane];
  focus: 0 | 1;
  hostRows: HostListRow[];
  collapsed: Record<string, boolean>;
  marks: [Record<string, boolean>, Record<string, boolean>];
  info: boolean;
  preview: boolean;
  previewOffset: number;
  onFocus: (pane: 0 | 1) => void;
  onOpenPickerRow: (pane: 0 | 1, cursor: number) => void;
  onSelectBrowser: (pane: 0 | 1, cursor: number) => void;
  onToggleMark: (pane: 0 | 1, name: string) => void;
  onToggleInfo: () => void;
  onTogglePreview: () => void;
  roomy?: boolean;
  footerDesktop: string;
  footerMobile: string;
}) {
  const previewEntries = browserEntries(panes[focus]);
  const previewEntry = previewEntries
    ? previewEntries[clampIndex(panes[focus].cursor, previewEntries.length)]
    : undefined;
  const side = (index: 0 | 1, compact: boolean, className?: string) => (
    <FileSide
      index={index}
      pane={panes[index]}
      focused={focus === index}
      hostRows={hostRows}
      collapsed={collapsed}
      marks={marks[index]}
      info={info}
      roomy={roomy}
      compact={compact}
      className={className}
      onFocus={onFocus}
      onOpenPickerRow={onOpenPickerRow}
      onSelectBrowser={onSelectBrowser}
      onToggleMark={onToggleMark}
      onToggleInfo={onToggleInfo}
    />
  );
  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      <div className={`h-px shrink-0 bg-border ${bleedMargin}`} />

      <div className="flex min-h-0 flex-1 flex-col gap-2 pt-2 md:hidden">
        {side(0, true)}
        <div className={`h-px shrink-0 bg-border ${bleedMargin}`} />
        {side(1, true)}
      </div>

      <div className="relative hidden min-h-0 flex-1 pt-2 md:block">
        <div className="grid h-full min-h-0 grid-cols-2">
          {side(0, false, "min-h-0 overflow-y-auto pr-3")}
          {side(
            1,
            false,
            `min-h-0 overflow-y-auto pl-3 ${footerDesktop ? "pb-10" : ""}`,
          )}
        </div>
        <div
          aria-hidden
          className="pointer-events-none absolute top-0 bottom-0 left-1/2 z-10 w-px bg-border"
        />
        {footerDesktop ? (
          <footer className="pointer-events-none absolute bottom-0 left-0 right-[var(--demo-gutter,0px)] pb-5 text-left text-[10px] text-muted sm:text-xs md:text-sm">
            {footerDesktop}
          </footer>
        ) : null}
      </div>

      {footerMobile ? (
        <footer className="mt-2 shrink-0 text-left text-[10px] text-muted sm:text-xs md:hidden">
          {footerMobile}
        </footer>
      ) : null}

      {preview && previewEntry && !previewEntry.isDir ? (
        <FilesPreview
          entry={previewEntry}
          offset={previewOffset}
          onClose={onTogglePreview}
        />
      ) : null}
    </div>
  );
}

function FileSide({
  index,
  pane,
  focused,
  hostRows,
  collapsed,
  marks,
  info,
  roomy,
  compact,
  className,
  onFocus,
  onOpenPickerRow,
  onSelectBrowser,
  onToggleMark,
  onToggleInfo,
}: {
  index: 0 | 1;
  pane: FilePane;
  focused: boolean;
  hostRows: HostListRow[];
  collapsed: Record<string, boolean>;
  marks: Record<string, boolean>;
  info: boolean;
  roomy: boolean;
  compact: boolean;
  className?: string;
  onFocus: (pane: 0 | 1) => void;
  onOpenPickerRow: (pane: 0 | 1, cursor: number) => void;
  onSelectBrowser: (pane: 0 | 1, cursor: number) => void;
  onToggleMark: (pane: 0 | 1, name: string) => void;
  onToggleInfo: () => void;
}) {
  if (pane.mode === "picker") {
    return (
      <FilesPicker
        cursor={clampIndex(pane.cursor, 1 + hostRows.length)}
        focused={focused}
        hostRows={hostRows}
        collapsed={collapsed}
        roomy={roomy}
        compact={compact}
        className={className}
        onFocus={() => onFocus(index)}
        onOpen={(cursor) => onOpenPickerRow(index, cursor)}
      />
    );
  }
  const entries = pane.mode === "local" ? localFiles : remoteFiles;
  const host = pane.mode === "remote" ? hostById(hostRows, pane.hostId) : undefined;
  const label =
    pane.mode === "local" ? "local" : (host?.alias ?? host?.name ?? pane.hostId);
  return (
    <FilesBrowser
      label={label}
      path={pane.mode === "local" ? "~" : "/"}
      tone={pane.mode === "local" ? localTone : remoteTone}
      focused={focused}
      entries={entries}
      cursor={clampIndex(pane.cursor, entries.length)}
      marked={pane.mode === "local" ? marks : undefined}
      info={info && focused}
      roomy={roomy}
      compact={compact}
      className={className}
      onFocus={() => onFocus(index)}
      onSelect={(cursor) => onSelectBrowser(index, cursor)}
      onToggleMark={
        pane.mode === "local" ? (name) => onToggleMark(index, name) : undefined
      }
      onToggleInfo={onToggleInfo}
    />
  );
}

function FilesPicker({
  cursor,
  focused,
  hostRows,
  collapsed,
  roomy,
  compact,
  className,
  onFocus,
  onOpen,
}: {
  cursor: number;
  focused: boolean;
  hostRows: HostListRow[];
  collapsed: Record<string, boolean>;
  roomy: boolean;
  compact: boolean;
  className?: string;
  onFocus: () => void;
  onOpen: (cursor: number) => void;
}) {
  const listRef = useRef<HTMLUListElement>(null);
  useEffect(() => {
    if (!focused) return;
    scrollSelectedInto(listRef.current);
  }, [cursor, focused, hostRows]);
  const listClass = compact ? "max-h-44 overflow-y-auto" : className;
  return (
    <ul
      ref={listRef}
      role="listbox"
      aria-label="Choose a host"
      className={listClass}
    >
      <li role="option" aria-selected={focused && cursor === 0}>
        <button
          type="button"
          onClick={() => {
            onFocus();
            onOpen(0);
          }}
          className={fileRowClass(focused && cursor === 0, roomy, true)}
        >
          {"   "}Local
        </button>
      </li>
      {hostRows.map((row, index) => {
        const selected = focused && cursor === index + 1;
        const pad =
          row.depth > 0 ? { paddingLeft: `${row.depth * 0.95}rem` } : undefined;
        if (row.kind === "group") {
          const indicator = collapsed[row.id] ? "▸" : "▾";
          return (
            <li key={row.id} role="option" aria-selected={selected}>
              <button
                type="button"
                onClick={() => onOpen(index + 1)}
                style={pad}
                className={`w-full cursor-pointer truncate whitespace-nowrap border-0 text-left font-[inherit] font-bold transition-colors ${
                  roomy ? "py-1.5" : "py-0.5"
                } ${
                  selected
                    ? "bg-highlight text-foreground"
                    : "bg-transparent text-accent hover:bg-highlight/70"
                }`}
              >
                {indicator}{" "}
                {row.googleCloud ? (
                  <ProviderTitle provider="gcp" />
                ) : row.label === "Amazon EC2" ? (
                  <ProviderTitle provider="aws" />
                ) : row.label === "Microsoft Azure" ? (
                  <ProviderTitle provider="azure" />
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
          <li key={row.id} role="option" aria-selected={selected}>
            <button
              type="button"
              onClick={() => onOpen(index + 1)}
              style={pad}
              className={fileRowClass(selected, roomy, true)}
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

function fileRowClass(selected: boolean, roomy: boolean, preserveSpace: boolean): string {
  return `w-full cursor-pointer truncate border-0 text-left font-[inherit] transition-colors ${
    preserveSpace ? "whitespace-pre" : "whitespace-nowrap"
  } ${roomy ? "py-1.5" : "py-0.5"} ${
    selected
      ? "bg-highlight font-bold text-foreground"
      : "bg-transparent text-foreground hover:bg-highlight/70"
  }`;
}

function FilesBrowser({
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
  roomy = false,
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
  roomy?: boolean;
  compact?: boolean;
  className?: string;
}) {
  const listRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!focused || info) return;
    scrollSelectedInto(listRef.current);
  }, [cursor, entries, focused, info]);
  const marker = focused ? "›" : " ";
  const entry = entries[cursor];
  const rootClass = compact ? "max-h-44 overflow-y-auto" : className;
  return (
    <div ref={listRef} className={rootClass}>
      <button
        type="button"
        onClick={onFocus}
        className="mb-1 w-full cursor-pointer truncate border-0 bg-transparent p-0 text-left font-[inherit]"
      >
        <span>{marker} </span>
        <span className={focused ? "font-bold" : ""} style={{ color: tone }}>
          {label}  {path}
        </span>
      </button>
      {info && entry ? (
        <FilesInfo entry={entry} onClose={onToggleInfo} />
      ) : entries.length === 0 ? (
        <p className="truncate pl-2 text-muted">Empty · a new folder · / jump</p>
      ) : (
        <ul role="listbox" aria-label={`${label} files`}>
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
                  className={fileRowClass(isSelected, roomy, true)}
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

function FilesPreview({
  entry,
  offset,
  onClose,
}: {
  entry: FileEntry;
  offset: number;
  onClose: () => void;
}) {
  const kind = entry.name.endsWith(".json") ? "json" : "text";
  const body = filePreviewBodies[entry.name] ?? "";
  const lines = body === "" ? [] : body.split("\n");
  const visible = lines.slice(offset, offset + previewVisibleLines);
  const title = `${entry.name} · ${kind} · ${entry.size}`;
  return (
    <button
      type="button"
      onClick={onClose}
      className="absolute inset-0 z-20 flex cursor-pointer items-center justify-center border-0 bg-transparent p-3 font-[inherit] text-left"
      aria-label="Close preview"
    >
      <div className="max-h-[80%] w-[min(100%,28rem)] overflow-hidden rounded-md border border-border bg-background px-3 py-2">
        <p className="truncate font-bold text-accent">{title}</p>
        {visible.length > 0 ? (
          <pre className="mt-2 max-h-40 overflow-y-auto whitespace-pre-wrap text-foreground">
            {visible.join("\n")}
          </pre>
        ) : null}
      </div>
    </button>
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
  roomy = false,
  onSelect,
  onToggleGroup,
  className,
}: {
  rows: HostListRow[];
  cursor: number;
  collapsed: Record<string, boolean>;
  roomy?: boolean;
  onSelect: (index: number) => void;
  onToggleGroup: (groupId: string) => void;
  className?: string;
}) {
  const listRef = useRef<HTMLUListElement>(null);
  useEffect(() => {
    const list = listRef.current;
    const item = list?.querySelector<HTMLElement>('[aria-selected="true"]');
    if (!list || !item) return;
    const listRect = list.getBoundingClientRect();
    const itemRect = item.getBoundingClientRect();
    if (itemRect.top < listRect.top) {
      list.scrollTop -= listRect.top - itemRect.top;
    } else if (itemRect.bottom > listRect.bottom) {
      list.scrollTop += itemRect.bottom - listRect.bottom;
    }
  }, [cursor, rows]);

  return (
    <ul ref={listRef} className={className} role="listbox" aria-label="hosts">
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
                className={`w-full cursor-pointer truncate whitespace-nowrap border-0 text-left font-[inherit] font-bold transition-colors ${
                  roomy ? "py-1.5" : "py-0.5"
                } ${
                  isSelected
                    ? "bg-highlight text-foreground"
                    : "bg-transparent text-accent hover:bg-highlight/70"
                }`}
              >
                {indicator}{" "}
                {row.googleCloud ? (
                  <ProviderTitle provider="gcp" />
                ) : row.label === "Amazon EC2" ? (
                  <ProviderTitle provider="aws" />
                ) : row.label === "Microsoft Azure" ? (
                  <ProviderTitle provider="azure" />
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
              className={`w-full cursor-pointer truncate whitespace-pre border-0 text-left font-[inherit] transition-colors ${
                roomy ? "py-1.5" : "py-0.5"
              } ${
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
  roomy = false,
  onSelect,
  className,
}: {
  items: KeyItem[];
  cursor: number;
  roomy?: boolean;
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
              className={`w-full cursor-pointer truncate whitespace-nowrap border-0 text-left font-[inherit] transition-colors ${
                roomy ? "py-1.5" : "py-0.5"
              } ${
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
      {footerDesktop ? (
        <footer className="mt-2 hidden shrink-0 pr-[var(--demo-gutter,0px)] text-left text-[10px] text-muted sm:text-xs md:block md:text-sm">
          {footerDesktop}
        </footer>
      ) : null}
      {footerMobile ? (
        <footer className="mt-2 shrink-0 text-left text-[10px] text-muted sm:text-xs md:hidden">
          {footerMobile}
        </footer>
      ) : null}
    </div>
  );
}

function SyncPanel({
  listIndex,
  detail,
  detailCursor,
  onSelectProvider,
  onSelectSetting,
  roomy = false,
  footerDesktop,
  footerMobile,
}: {
  listIndex: number;
  detail: boolean;
  detailCursor: number;
  onSelectProvider: (index: number) => void;
  onSelectSetting: (index: number) => void;
  roomy?: boolean;
  footerDesktop: string;
  footerMobile: string;
}) {
  const provider = syncProviders[listIndex];
  if (!provider) return null;
  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      <div className={`h-px shrink-0 bg-border ${bleedMargin}`} />

      <div className="flex min-h-0 flex-1 flex-col md:hidden">
        <SyncList
          listIndex={listIndex}
          roomy={roomy}
          onSelect={onSelectProvider}
          className="max-h-[42%] shrink-0 overflow-y-auto pt-1.5"
        />
        <div className={`my-1.5 h-px shrink-0 bg-border ${bleedMargin}`} />
        <SyncDetail
          provider={provider}
          detail={detail}
          cursor={detailCursor}
          roomy={roomy}
          onSelect={onSelectSetting}
          className="min-h-0 flex-1 overflow-y-auto pt-0.5"
        />
      </div>

      <div className="relative hidden min-h-0 flex-1 md:block">
        <div
          aria-hidden
          className={`pointer-events-none absolute top-0 h-px bg-border ${bleedX}`}
        />
        <div className={`${panelGrid} md:grid`}>
          <SyncList
            listIndex={listIndex}
            roomy={roomy}
            onSelect={onSelectProvider}
            className="min-h-0 overflow-y-auto pt-1.5"
          />
          <SyncDetail
            provider={provider}
            detail={detail}
            cursor={detailCursor}
            roomy={roomy}
            onSelect={onSelectSetting}
            className={`min-h-0 overflow-y-auto pt-1.5 pl-3 md:pl-4 ${
              footerDesktop ? "pb-10" : "pb-3"
            }`}
          />
        </div>
        <div
          aria-hidden
          className="pointer-events-none absolute top-0 bottom-0 left-[38%] z-10 w-px bg-border"
        />
        {footerDesktop ? (
          <footer className="pointer-events-none absolute bottom-0 left-0 right-[var(--demo-gutter,0px)] pb-5 pl-3 text-left text-[10px] text-muted sm:text-xs md:text-sm">
            {footerDesktop}
          </footer>
        ) : null}
      </div>

      {footerMobile ? (
        <footer className="mt-2 shrink-0 text-left text-[10px] text-muted sm:text-xs md:hidden">
          {footerMobile}
        </footer>
      ) : null}
    </div>
  );
}

function SyncList({
  listIndex,
  roomy,
  onSelect,
  className,
}: {
  listIndex: number;
  roomy: boolean;
  onSelect: (index: number) => void;
  className?: string;
}) {
  const listRef = useRef<HTMLUListElement>(null);
  useEffect(() => {
    scrollSelectedInto(listRef.current);
  }, [listIndex]);
  return (
    <ul ref={listRef} role="listbox" aria-label="providers" className={className}>
      {syncProviders.map((provider, index) => {
        const selected = index === listIndex;
        return (
          <li key={provider.id} role="option" aria-selected={selected}>
            <button
              type="button"
              onClick={() => onSelect(index)}
              className={`flex w-full min-w-0 cursor-pointer items-center gap-3 border-0 text-left font-[inherit] transition-colors ${
                roomy ? "py-1.5" : "py-0.5"
              } ${
                selected
                  ? "bg-highlight"
                  : "bg-transparent hover:bg-highlight/70"
              }`}
            >
              <span className="min-w-0 flex-1 truncate pl-1">
                <ProviderTitle provider={provider.id} />
              </span>
              <span className="shrink-0 pr-2">
                <span className={syncValueClass(provider.status)}>
                  {provider.status}
                </span>
                {provider.status === "on" && provider.count ? (
                  <span className="text-foreground"> ({provider.count})</span>
                ) : null}
              </span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}

function SyncDetail({
  provider,
  detail,
  cursor,
  roomy,
  onSelect,
  className,
}: {
  provider: SyncProviderModel;
  detail: boolean;
  cursor: number;
  roomy: boolean;
  onSelect: (index: number) => void;
  className?: string;
}) {
  const rows = syncDetailRows(provider);
  const listRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!detail) return;
    scrollSelectedInto(listRef.current);
  }, [cursor, detail, provider]);
  const blocks: ReactNode[] = [];
  let previous = "";
  rows.forEach((row, index) => {
    if (row.section !== previous) {
      if (previous) {
        blocks.push(<div key={`gap-${row.section}`} className="h-3" />);
      }
      blocks.push(<SyncSection key={row.section} title={row.section} />);
      previous = row.section;
    }
    const selected = detail && index === cursor;
    blocks.push(
      <button
        key={`${row.section}-${row.label}`}
        type="button"
        role="option"
        aria-selected={selected}
        onClick={() => onSelect(index)}
        className={`flex w-full min-w-0 cursor-pointer items-baseline gap-3 border-0 text-left font-[inherit] transition-colors ${
          roomy ? "py-1.5" : "py-0.5"
        } ${
          selected
            ? "bg-highlight font-bold text-foreground"
            : "bg-transparent text-foreground hover:bg-highlight/70"
        }`}
      >
        <span className="min-w-0 flex-1 truncate pl-2">{row.label}</span>
        {row.value ? (
          <span className={`shrink-0 pr-2 font-normal ${syncValueClass(row.value)}`}>
            {row.value}
          </span>
        ) : null}
      </button>,
    );
  });
  return (
    <div ref={listRef} className={className}>
      <p className="truncate pl-2">
        <ProviderTitle provider={provider.id} />
      </p>
      {provider.error ? (
        <p className="mt-2 px-2 text-[#EF4444]">{provider.error}</p>
      ) : null}
      <div
        className="mt-2"
        role="listbox"
        aria-label={`${providerLabel[provider.id]} settings`}
      >
        {blocks}
      </div>
    </div>
  );
}

function SyncSection({ title }: { title: string }) {
  return (
    <p className="flex min-w-0 items-center pl-2 text-muted">
      <span className="shrink-0">{title} </span>
      <span aria-hidden className="min-w-0 flex-1 overflow-hidden whitespace-nowrap">
        {"─".repeat(80)}
      </span>
    </p>
  );
}

function VaultPanel({
  cursor,
  onSelectChip,
  onSelectMenu,
  roomy = false,
  footerDesktop,
  footerMobile,
}: {
  cursor: number;
  onSelectChip: () => void;
  onSelectMenu: (index: number) => void;
  roomy?: boolean;
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
            roomy={roomy}
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
  roomy = false,
  onSelect,
}: {
  item: SyncMenuItem;
  selected: boolean;
  roomy?: boolean;
  onSelect: () => void;
}) {
  return (
    <li role="option" aria-selected={selected}>
      <button
        type="button"
        disabled={item.disabled}
        onClick={onSelect}
        className={`flex w-full items-baseline gap-4 border-0 text-left font-[inherit] transition-colors ${
          roomy ? "py-1.5" : "py-0.5"
        } ${
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

function ProviderGlyph({
  provider,
}: {
  provider: Exclude<SyncProvider, "">;
}) {
  return (
    <span
      className={`${nerdSymbols.className} mr-1 inline-block w-[1ch] shrink-0`}
      aria-hidden
    >
      {providerNerdIcon[provider]}
    </span>
  );
}

function ProviderTitle({
  provider,
}: {
  provider: Exclude<SyncProvider, "">;
}) {
  const glyph = <ProviderGlyph provider={provider} />;
  if (provider === "gcp") {
    return (
      <span className="inline-flex items-baseline font-bold">
        <span className={providerColor.gcp}>{glyph}</span>
        <GoogleCloudLabel />
      </span>
    );
  }
  return (
    <span className={`inline-flex items-baseline font-bold ${providerColor[provider]}`}>
      {glyph}
      {providerLabel[provider]}
    </span>
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
