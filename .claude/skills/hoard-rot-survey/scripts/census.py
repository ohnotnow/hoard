#!/usr/bin/env python3
"""Tier 0 census: size\tpath TSV over a mount. Metadata only, no media reads."""
import os
import sys

VIDEO_EXTS = {
    ".avi", ".divx", ".flv", ".m4v", ".mkv", ".mov", ".mp4",
    ".mpeg", ".mpg", ".ts", ".webm", ".wmv",
}

root, out_path = sys.argv[1], sys.argv[2]
files = 0
total = 0
video = 0
zero_video = []
with open(out_path, "w") as out:
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in (".Spotlight-V100", ".Trashes", ".fseventsd")]
        for name in filenames:
            full = os.path.join(dirpath, name)
            try:
                size = os.lstat(full).st_size
            except OSError as e:
                out.write(f"STATFAIL:{e.errno}\t{full}\n")
                continue
            out.write(f"{size}\t{full}\n")
            files += 1
            total += size
            ext = os.path.splitext(name)[1].lower()
            if ext in VIDEO_EXTS and not name.startswith("._"):
                video += 1
                if size == 0:
                    zero_video.append(full)

print(f"files: {files}")
print(f"bytes: {total} ({total / 1e12:.2f} TB)")
print(f"video files (non-AppleDouble): {video}")
print(f"zero-byte video files: {len(zero_video)}")
for p in zero_video[:20]:
    print(f"  ZERO: {p}")
