export const comparisonNavItems = [
  {
    href: "/termius",
    label: "Termius",
    blurb: "GUI SSH client",
  },
  {
    href: "/putty",
    label: "PuTTY",
    blurb: "Classic Windows client",
  },
  {
    href: "/mobaxterm",
    label: "MobaXterm",
    blurb: "Windows toolbox",
  },
  {
    href: "/securecrt",
    label: "SecureCRT",
    blurb: "Commercial GUI client",
  },
] as const;

export const guideNavItems = [
  {
    href: "/ssh-host-manager",
    label: "SSH host manager",
    blurb: "Browse and organize hosts",
  },
  {
    href: "/sync-ssh-hosts",
    label: "Sync SSH hosts",
    blurb: "Encrypted vault between machines",
  },
  {
    href: "/cloud-ssh",
    label: "Cloud SSH",
    blurb: "GCP, AWS, Azure, Hetzner, and Box",
  },
  {
    href: "/ssh-sftp",
    label: "SSH SFTP",
    blurb: "Dual-pane file transfers",
  },
  {
    href: "/ssh-key-manager",
    label: "SSH key manager",
    blurb: "Generate and install keys",
  },
] as const;

export const resourceNavItems = [
  { href: "/docs", label: "Docs" },
  { href: "/changelog", label: "Changelog" },
  { href: "/alternatives", label: "Comparisons" },
  { href: "/features", label: "Features" },
] as const;
