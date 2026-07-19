#!/usr/bin/env python3
"""Build a compact, read-only media filename catalogue.

The script never modifies source media. It accepts local roots and/or text files
containing one path per line, then writes analysis artifacts to --output-dir.
"""

from __future__ import annotations

import argparse
import json
import re
import unicodedata
from collections import Counter, defaultdict
from pathlib import Path

VIDEO_EXTS = {
    ".avi", ".divx", ".flv", ".m4v", ".mkv", ".mov", ".mp4",
    ".mpeg", ".mpg", ".ts", ".webm", ".wmv",
}
AUDIO_EXTS = {".aac", ".flac", ".m4a", ".mp3", ".ogg", ".wav"}
GENERIC_DEFAULTS = {
    "audio", "comedy", "complete", "documentaries", "documentary", "film",
    "films", "food", "media", "movies", "scifi", "sci fi",
    "storage", "torrents", "tv", "var", "video", "videos",
}

EPISODE_MARKER = re.compile(
    r"(?ix)\b(?:s\d{1,2}[ ._-]*e\d{1,3}|\d{1,2}x\d{1,3}|"
    r"(?:series|season)\s*[-.:]?\s*\d+|episodes?\s*[-.:]?\s*\d+|"
    r"eps?\s*[-.:]?\s*\d+)\b"
)
DATE_MARKER = re.compile(
    r"(?ix)\b(?:19|20)\d{2}[-./]\d{1,2}[-./]\d{1,2}\b|"
    r"\b\d{1,2}(?:st|nd|rd|th)?\s+"
    r"(?:jan\w*|feb\w*|mar\w*|apr\w*|may|jun\w*|jul\w*|aug\w*|"
    r"sep\w*|oct\w*|nov\w*|dec\w*)\s+(?:19|20)\d{2}\b"
)
TECH_MARKER = re.compile(
    r"(?ix)\b(?:2160p|1080p|720p|576p|540p|504p|480p|360p|352p|"
    r"uhd|fhd|hdtv|pdtv|webrip|web[- .]?dl|dvdrip|brrip|bluray|vhsrip|"
    r"x26[45]|h[ ._]?26[45]|xvid|divx|avc|hevc|aac|ac3|proper|repack|"
    r"remux|subs?|subbed|complete)\b"
)
STORAGE_COMPONENT = re.compile(
    r"(?i)^(?:sd[a-z]\d*(?:[-_].*)?|.*scsi.*|mnt|volumes?|home|users?)$"
)


def norm_key(text: str) -> str:
    value = unicodedata.normalize("NFKD", text).encode("ascii", "ignore").decode()
    value = value.lower().replace("&", " and ")
    value = re.sub(r"[^a-z0-9]+", " ", value)
    return re.sub(r"\s+", " ", value).strip()


def clean_title(text: str) -> str:
    value = unicodedata.normalize("NFKC", text).replace("_", " ")
    value = re.sub(r"\.(?=[A-Za-z])", " ", value)
    value = DATE_MARKER.sub(" ", value)

    match = EPISODE_MARKER.search(value)
    if match and match.start() > 1:
        value = value[: match.start()]

    value = re.sub(
        r"\s*[\[(](?:19|20)\d{2}(?:\s*[-–]\s*\d{2,4})?[^\])]*[\])]",
        " ", value, flags=re.I,
    )
    value = re.sub(
        r"\s*\[[^\]]*(?:rip|xvid|divx|\d+p\b|subs?|web|hdtv|pdtv)[^\]]*\]",
        " ", value, flags=re.I,
    )

    match = TECH_MARKER.search(value)
    if match and match.start() > 2:
        value = value[: match.start()]

    value = re.sub(r"(?i)\b(?:part|pt|disc|cd)\s*\d+\b.*$", " ", value)
    value = re.sub(r"(?i)^\s*(?:episode|ep|part)\s*\d+\s*[-.:]?\s*", " ", value)
    value = re.sub(r"(?i)^\s*\d{1,4}\s*[-.:]\s*", " ", value)
    value = re.sub(r"\s+", " ", value).strip(" ._-–[](){}:")
    return value


def generic_title(text: str, generic: set[str]) -> bool:
    key = norm_key(text)
    return (
        not key
        or key in generic
        or key.isdigit()
        or len(key) < 3
        or bool(STORAGE_COMPONENT.match(key))
        or bool(re.match(r"^(?:episode|ep|part|disc|cd)\s*\d+", key))
    )


def candidate_from_path(raw_path: str, generic: set[str]) -> str:
    path = Path(raw_path)
    stem = clean_title(path.stem)

    ancestors: list[str] = []
    for component in reversed(path.parts[:-1]):
        candidate = clean_title(component)
        if not generic_title(candidate, generic):
            ancestors.append(candidate)
    parent = ancestors[0] if ancestors else ""

    if generic_title(stem, generic):
        return parent

    stem_key = norm_key(stem)
    parent_key = norm_key(parent)
    if parent and (stem_key.startswith(parent_key) or parent_key.startswith(stem_key)):
        return parent if len(parent_key) <= len(stem_key) else stem

    if parent and re.match(r"(?i)^(?:[a-z]{1,8}\d+|\d+[a-z]?)$", stem):
        return parent

    return stem or parent


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", action="append", default=[], help="Local media root; repeatable")
    parser.add_argument(
        "--paths-file", action="append", default=[],
        help="Text file containing one media path per line; repeatable",
    )
    parser.add_argument("--output-dir", required=True, help="Directory for generated artifacts")
    parser.add_argument(
        "--generic-dir", action="append", default=[],
        help="Additional storage/taxonomy folder name to ignore; repeatable",
    )
    args = parser.parse_args()
    if not args.root and not args.paths_file:
        parser.error("provide at least one --root or --paths-file")
    return args


def main() -> int:
    args = parse_args()
    output_dir = Path(args.output_dir).expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    generic = {norm_key(item) for item in GENERIC_DEFAULTS}
    generic.update(norm_key(item) for item in args.generic_dir)

    records: list[tuple[str, str]] = []
    for root_text in args.root:
        root = Path(root_text).expanduser().resolve()
        if not root.is_dir():
            raise SystemExit(f"Not a directory: {root}")
        label = f"root:{root}"
        for path in root.rglob("*"):
            if path.is_file():
                records.append((str(path), label))

    for inventory_text in args.paths_file:
        inventory = Path(inventory_text).expanduser().resolve()
        if not inventory.is_file():
            raise SystemExit(f"Not a path inventory: {inventory}")
        label = f"inventory:{inventory.name}"
        for line in inventory.read_text(errors="replace").splitlines():
            if line.strip():
                records.append((line.strip(), label))

    # Stable deduplication preserves source identity while removing repeated input lines.
    records = list(dict.fromkeys(records))
    inventory_output = output_dir / "path_inventory.txt"
    inventory_output.write_text("\n".join(path for path, _ in records) + "\n")

    extensions = Counter(Path(path).suffix.lower() for path, _ in records)
    depths = Counter(len(Path(path).parts) for path, _ in records)
    rows: list[tuple[str, str, str, str]] = []
    year_mentions = Counter()

    for raw_path, source in records:
        suffix = Path(raw_path).suffix.lower()
        if suffix in VIDEO_EXTS:
            media_type = "video"
        elif suffix in AUDIO_EXTS:
            media_type = "audio"
        else:
            continue

        title = candidate_from_path(raw_path, generic)
        key = norm_key(title)
        if not key or generic_title(title, generic):
            continue
        rows.append((key, title, media_type, source))

        for year in re.findall(r"(?<!\d)((?:19|20)\d{2})(?!\d)", raw_path):
            numeric_year = int(year)
            if 1930 <= numeric_year <= 2029:
                year_mentions[numeric_year // 10 * 10] += 1

    grouped: dict[str, list[tuple[str, str, str]]] = defaultdict(list)
    for key, title, media_type, source in rows:
        grouped[key].append((title, media_type, source))

    catalogue = []
    for key, values in grouped.items():
        representative = Counter(title for title, _, _ in values).most_common(1)[0][0]
        video_count = sum(media_type == "video" for _, media_type, _ in values)
        audio_count = sum(media_type == "audio" for _, media_type, _ in values)
        source_count = len({source for _, _, source in values})
        catalogue.append((video_count + audio_count, representative, key, video_count, audio_count, source_count))
    catalogue.sort(key=lambda item: (-item[0], item[1].lower()))

    tsv = output_dir / "canonical_titles.tsv"
    tsv.write_text(
        "items\ttitle\tkey\tvideos\taudio\tsources\n"
        + "\n".join(
            f"{total}\t{title}\t{key}\t{video}\t{audio}\t{sources}"
            for total, title, key, video, audio, sources in catalogue
        )
        + "\n"
    )

    summary = {
        "input_paths": len(records),
        "media_items": len(rows),
        "video_items": sum(media_type == "video" for _, _, media_type, _ in rows),
        "audio_items": sum(media_type == "audio" for _, _, media_type, _ in rows),
        "candidate_titles": len(catalogue),
        "recurring_candidates": sum(total >= 2 for total, *_ in catalogue),
        "extensions": extensions.most_common(),
        "path_depths": sorted(depths.items()),
        "year_mentions_by_decade": sorted(year_mentions.items()),
        "top_candidates": [
            {
                "items": total,
                "title": title,
                "videos": video,
                "audio": audio,
                "sources": sources,
            }
            for total, title, _, video, audio, sources in catalogue[:150]
        ],
        "artifacts": {
            "canonical_titles": str(tsv),
            "path_inventory": str(inventory_output),
        },
        "warning": "Paths show acquisition/retention, not proof of watching or enjoyment.",
    }
    summary_path = output_dir / "summary.json"
    summary["artifacts"]["summary"] = str(summary_path)
    summary_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")

    print(json.dumps({
        "input_paths": summary["input_paths"],
        "video_items": summary["video_items"],
        "audio_items": summary["audio_items"],
        "candidate_titles": summary["candidate_titles"],
        "recurring_candidates": summary["recurring_candidates"],
        "output_dir": str(output_dir),
    }, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
