const BAST_REPO = "ellipse-software/bast";

export const bastRepoUrl = `https://github.com/${BAST_REPO}`;
export const bastWebRepoUrl = bastRepoUrl;
export const bastWebDocsPath = `${bastRepoUrl}/tree/master/apps/web/content/docs`;
export const bastReleasesUrl = `${bastRepoUrl}/releases`;
export const bastSponsorUrl = "https://github.com/sponsors/tedbrine";

type GitHubRelease = {
  tag_name?: string;
  name?: string | null;
  body?: string | null;
  html_url?: string;
  published_at?: string | null;
  prerelease?: boolean;
  draft?: boolean;
};

export type BastRelease = {
  tag: string;
  name: string;
  body: string;
  url: string;
  publishedAt: string | null;
  prerelease: boolean;
};

const githubHeaders = {
  Accept: "application/vnd.github+json",
  "User-Agent": "bast.sh",
} as const;

export async function getLatestBastVersion(): Promise<string | null> {
  try {
    const response = await fetch(
      `https://api.github.com/repos/${BAST_REPO}/releases/latest`,
      {
        headers: githubHeaders,
        next: { revalidate: 300 },
      },
    );

    if (!response.ok) {
      return null;
    }

    const release = (await response.json()) as GitHubRelease;
    return release.tag_name ?? null;
  } catch {
    return null;
  }
}

export async function getBastReleases(limit = 30): Promise<BastRelease[]> {
  try {
    const response = await fetch(
      `https://api.github.com/repos/${BAST_REPO}/releases?per_page=${limit}`,
      {
        headers: githubHeaders,
        next: { revalidate: 3600 },
      },
    );

    if (!response.ok) {
      return [];
    }

    const releases = (await response.json()) as GitHubRelease[];
    return releases
      .filter((release) => {
        if (release.draft || !release.tag_name) return false;
        if (release.prerelease) return false;
        if (release.tag_name.startsWith("nightly.")) return false;
        return true;
      })
      .map((release) => ({
        tag: release.tag_name as string,
        name: release.name?.trim() || (release.tag_name as string),
        body: (release.body ?? "").trim(),
        url: release.html_url || bastReleaseUrl(release.tag_name as string),
        publishedAt: release.published_at ?? null,
        prerelease: Boolean(release.prerelease),
      }));
  } catch {
    return [];
  }
}

export function bastReleaseUrl(version: string): string {
  return `https://github.com/${BAST_REPO}/releases/tag/${version}`;
}
