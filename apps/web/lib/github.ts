const BAST_REPO = "ellipse-software/bast";

export const bastRepoUrl = `https://github.com/${BAST_REPO}`;
export const bastWebRepoUrl = bastRepoUrl;
export const bastWebDocsPath = `${bastRepoUrl}/tree/master/apps/web/content/docs`;

type GitHubRelease = {
  tag_name?: string;
};

export async function getLatestBastVersion(): Promise<string | null> {
  try {
    const response = await fetch(
      `https://api.github.com/repos/${BAST_REPO}/releases/latest`,
      {
        headers: {
          Accept: "application/vnd.github+json",
          "User-Agent": "bast.sh",
        },
        next: { revalidate: 3600 },
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

export function bastReleaseUrl(version: string): string {
  return `https://github.com/${BAST_REPO}/releases/tag/${version}`;
}
