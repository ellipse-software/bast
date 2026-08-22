import { createMarkdownRenderer } from "fumadocs-core/content/md";
import { remarkGfm } from "fumadocs-core/mdx-plugins/remark-gfm";

import { bastRepoUrl } from "@/lib/github";

const bastGithubPrefix = `${bastRepoUrl}/`;
const skipParents = new Set([
  "code",
  "inlineCode",
  "link",
  "linkReference",
  "definition",
  "html",
]);

type MdNode = {
  type: string;
  value?: string;
  url?: string;
  children?: MdNode[];
};

export type GithubTextPart =
  | { type: "text"; value: string }
  | { type: "link"; value: string; url: string };

export function shortenBastGithubUrl(url: string): string | null {
  if (!url.startsWith(bastGithubPrefix)) return null;

  const path = url.slice(bastGithubPrefix.length).replace(/\/$/, "");
  if (!path) return null;

  const issue = /^(?:issues|pull)\/(\d+)(?:[#?].*)?$/.exec(path);
  if (issue) return `#${issue[1]}`;

  return path;
}

export function linkifyGithubText(value: string): GithubTextPart[] {
  const parts: GithubTextPart[] = [];
  let last = 0;
  const pattern =
    /@([a-zA-Z0-9](?:[a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?)(?:\[bot\])?|#([1-9][0-9]{0,6})(?![0-9])/g;

  for (const match of value.matchAll(pattern)) {
    const index = match.index ?? 0;
    const previous = index > 0 ? value[index - 1] : "";
    if (previous && /[A-Za-z0-9/_]/.test(previous)) continue;

    if (index > last) {
      parts.push({ type: "text", value: value.slice(last, index) });
    }

    const mention = match[1];
    const issue = match[2];
    if (mention) {
      const isBot = match[0].endsWith("[bot]");
      parts.push({
        type: "link",
        value: match[0],
        url: isBot
          ? `https://github.com/apps/${mention}`
          : `https://github.com/${mention}`,
      });
    } else if (issue) {
      parts.push({
        type: "link",
        value: match[0],
        url: `${bastRepoUrl}/issues/${issue}`,
      });
    }

    last = index + match[0].length;
  }

  if (last < value.length) {
    parts.push({ type: "text", value: value.slice(last) });
  }

  return parts.length > 0 ? parts : [{ type: "text", value }];
}

function partsToNodes(parts: GithubTextPart[]): MdNode[] {
  return parts.map((part) =>
    part.type === "text"
      ? { type: "text", value: part.value }
      : {
          type: "link",
          url: part.url,
          children: [{ type: "text", value: part.value }],
        },
  );
}

function linkText(node: MdNode): string | null {
  if (!node.children || node.children.length !== 1) return null;
  const child = node.children[0];
  if (!child || child.type !== "text" || child.value === undefined) return null;
  return child.value;
}

function shortenGithubLink(node: MdNode) {
  if (!node.url) return;
  const text = linkText(node);
  if (text !== node.url) return;
  const short = shortenBastGithubUrl(node.url);
  const child = node.children?.[0];
  if (!short || !child) return;
  child.value = short;
}

function transform(node: MdNode) {
  if (node.type === "link") {
    shortenGithubLink(node);
    return;
  }

  if (skipParents.has(node.type) || !node.children) return;

  const children = node.children;
  for (let index = 0; index < children.length; index += 1) {
    const child = children[index];
    if (!child) continue;

    if (child.type === "text" && child.value) {
      const parts = linkifyGithubText(child.value);
      if (parts.length === 1 && parts[0]?.type === "text") {
        continue;
      }
      children.splice(index, 1, ...partsToNodes(parts));
      index += parts.length - 1;
      continue;
    }

    transform(child);
  }
}

export function remarkChangelogGithub() {
  return (tree: MdNode) => {
    transform(tree);
  };
}

export const changelogMarkdownRenderer = createMarkdownRenderer({
  remarkPlugins: [remarkGfm, remarkChangelogGithub],
});
