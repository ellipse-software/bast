export type TweetTextPart =
  | { type: "text"; value: string }
  | { type: "mention"; value: string };

const MENTION = /@[A-Za-z0-9_]{1,15}(?![A-Za-z0-9_])/g;

export function splitTweetText(text: string): TweetTextPart[] {
  const parts: TweetTextPart[] = [];
  let last = 0;

  for (const match of text.matchAll(MENTION)) {
    const index = match.index ?? 0;
    const previous = index > 0 ? text[index - 1] : "";
    if (previous && /[A-Za-z0-9.]/.test(previous)) continue;

    if (index > last) {
      parts.push({ type: "text", value: text.slice(last, index) });
    }
    parts.push({ type: "mention", value: match[0] });
    last = index + match[0].length;
  }

  if (last < text.length) {
    parts.push({ type: "text", value: text.slice(last) });
  }

  return parts.length > 0 ? parts : [{ type: "text", value: text }];
}

export function shuffle<T>(items: readonly T[]): T[] {
  const next = [...items];
  for (let i = next.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1));
    const current = next[i];
    const swap = next[j];
    if (current === undefined || swap === undefined) continue;
    next[i] = swap;
    next[j] = current;
  }
  return next;
}
