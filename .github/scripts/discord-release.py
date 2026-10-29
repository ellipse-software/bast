#!/usr/bin/env python3
"""Build a Discord webhook payload from a Bast changelog section."""

from __future__ import annotations

import argparse
import json
import re
import sys

# https://discord.com/developers/docs/resources/message#embed-object-embed-limits
COLOR = 0x8B5CF6
TITLE_MAX = 256
DESCRIPTION_MAX = 4096
FOOTER_MAX = 2048
CONTENT_MAX = 2000
EMBED_TOTAL_MAX = 6000
MAX_EMBEDS = 10

GET_STARTED = re.compile(
    r"^Get started at (?:\[https://bast\.sh\]\(https://bast\.sh\)|https://bast\.sh) "
    r"or update with `bast update`\.\s*$"
)
HEADING = re.compile(r"^#{2,3}\s+(.+?)\s*$")
TAG = re.compile(r"^v[0-9][^\s/]*$")
REPO = re.compile(r"^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")
DATE = re.compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}$")
FOOTER_CTA = "Get started at bast.sh or update with bast update"
LINK_BUTTON = 5
ACTION_ROW = 1


def with_ellipsis(value: str, limit: int) -> str:
    value = value.rstrip()
    if len(value) <= limit:
        return value
    if limit <= 1:
        return "…"[:limit]
    return value[: limit - 1].rstrip() + "…"


def cut_point(line: str, limit: int) -> int:
    if len(line) <= limit:
        return len(line)
    window = line[:limit]
    space = window.rfind(" ")
    if space < limit // 2:
        return limit
    return space


def peel_get_started(notes: str) -> tuple[str, bool]:
    text = notes.strip("\n")
    if not text:
        return "", False
    lines = text.split("\n")
    if GET_STARTED.match(lines[-1].strip()):
        return "\n".join(lines[:-1]).strip("\n"), True
    return text, False


def parse_sections(notes: str) -> tuple[str, list[tuple[str, str]]]:
    preamble: list[str] = []
    sections: list[tuple[str, list[str]]] = []
    current: tuple[str, list[str]] | None = None
    in_fence = False
    for line in notes.split("\n"):
        if line.strip().startswith("```"):
            in_fence = not in_fence
            target = preamble if current is None else current[1]
            target.append(line)
            continue
        heading = None if in_fence else HEADING.match(line)
        if heading is not None:
            current = (heading.group(1).strip(), [])
            sections.append(current)
            continue
        if current is None:
            preamble.append(line)
        else:
            current[1].append(line)
    description = "\n".join(preamble).strip()
    fields = []
    for name, body in sections:
        text = "\n".join(body).strip()
        if name and text:
            fields.append((name, text))
    return description, fields


def format_notes(preamble: str, sections: list[tuple[str, str]]) -> str:
    parts: list[str] = []
    if preamble:
        parts.append(preamble)
    for name, text in sections:
        parts.append(f"**{name}**\n{text}")
    return "\n\n".join(parts)


def split_description(text: str) -> list[str]:
    if not text:
        return []
    if len(text) <= DESCRIPTION_MAX:
        return [text]

    chunks: list[str] = []
    current = ""

    def flush() -> None:
        nonlocal current
        if current.strip():
            chunks.append(current)
        current = ""

    for line in text.split("\n"):
        while True:
            separator = "\n" if current else ""
            room = DESCRIPTION_MAX - len(current) - len(separator)
            if room >= len(line) and room > 0:
                current = f"{current}{separator}{line}"
                break
            if room >= 40:
                cut = cut_point(line, room)
                piece = line[:cut].rstrip()
                if piece:
                    current = f"{current}{separator}{piece}"
                    line = line[cut:].lstrip()
                    flush()
                    continue
            if current:
                flush()
                continue
            cut = cut_point(line, DESCRIPTION_MAX)
            current = line[:cut].rstrip()
            line = line[cut:].lstrip()
            flush()
    flush()
    return chunks


def fit_descriptions(chunks: list[str], overhead: int) -> list[str]:
    fitted: list[str] = []
    used = overhead
    for chunk in chunks:
        if len(fitted) >= MAX_EMBEDS:
            break
        if used + len(chunk) <= EMBED_TOTAL_MAX:
            fitted.append(chunk)
            used += len(chunk)
            continue
        room = EMBED_TOTAL_MAX - used
        if room >= 20 and len(fitted) < MAX_EMBEDS:
            fitted.append(with_ellipsis(chunk, room))
        elif fitted:
            fitted[-1] = with_ellipsis(fitted[-1], max(1, len(fitted[-1]) - 1))
        break
    return fitted


def release_url(repo: str, tag: str) -> str:
    return f"https://github.com/{repo}/releases/tag/{tag}"


def link_buttons(repo: str, tag: str) -> list[dict]:
    links = (
        ("GitHub", release_url(repo, tag)),
        ("Changelog", "https://bast.sh/changelog"),
        ("Website", "https://bast.sh"),
    )
    return [
        {
            "type": ACTION_ROW,
            "components": [
                {"type": 2, "style": LINK_BUTTON, "label": label, "url": url}
                for label, url in links
            ],
        }
    ]


def footer_text(date: str, get_started: bool) -> str:
    parts: list[str] = []
    if date:
        parts.append(date)
    if get_started:
        parts.append(FOOTER_CTA)
    return with_ellipsis(" · ".join(parts), FOOTER_MAX)


def build_payload(tag: str, repo: str, notes: str, date: str, content: str) -> dict:
    if not TAG.match(tag):
        raise SystemExit(f"tag must match v* with a leading digit (got: {tag})")
    if not REPO.match(repo):
        raise SystemExit(f"repository must look like owner/name (got: {repo})")
    if date and not DATE.match(date):
        raise SystemExit(f"changelog date must be YYYY-MM-DD (got: {date})")
    body, get_started = peel_get_started(notes)
    if not body and not get_started:
        raise SystemExit("release notes are empty")

    preamble, sections = parse_sections(body)
    title = with_ellipsis(f"Bast {tag}", TITLE_MAX)
    footer = footer_text(date, get_started)
    chunks = fit_descriptions(split_description(format_notes(preamble, sections)), len(title) + len(footer))
    if not chunks and not footer:
        raise SystemExit("release notes are empty")

    url = release_url(repo, tag)
    embeds: list[dict] = []
    if not chunks:
        embeds.append({"color": COLOR, "title": title, "url": url})
    for index, chunk in enumerate(chunks):
        embed: dict = {"color": COLOR, "description": chunk}
        if index == 0:
            embed["title"] = title
            embed["url"] = url
        embeds.append(embed)
    if footer:
        embeds[-1]["footer"] = {"text": footer}

    payload: dict = {
        "username": "Bast",
        "allowed_mentions": {"parse": []},
        "embeds": embeds,
        "components": link_buttons(repo, tag),
    }
    if content.strip():
        payload["content"] = with_ellipsis(content.strip(), CONTENT_MAX)
    return payload


def main() -> None:
    parser = argparse.ArgumentParser(description="Build a Bast release Discord payload")
    parser.add_argument("--tag", required=True)
    parser.add_argument("--repo", required=True)
    parser.add_argument("--date", default="")
    parser.add_argument("--content", default="")
    args = parser.parse_args()
    notes = sys.stdin.read()
    payload = build_payload(args.tag, args.repo, notes, args.date, args.content)
    json.dump(payload, sys.stdout, ensure_ascii=False)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
