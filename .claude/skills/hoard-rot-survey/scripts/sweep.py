#!/usr/bin/env python3
"""Tier 1 structural sweep: ffprobe container-parse over every video file.

Reads the census TSV, probes each video-ext file (skipping AppleDouble ._*),
writes a verdict TSV. Resumable: already-probed paths are skipped on re-run.
Read-only towards the media; output lives outside the mount.
"""
import json
import os
import subprocess
import sys
import threading
import queue

VIDEO_EXTS = {
    ".avi", ".divx", ".flv", ".m4v", ".mkv", ".mov", ".mp4",
    ".mpeg", ".mpg", ".ts", ".webm", ".wmv",
}
WORKERS = 3

census_path, out_path = sys.argv[1], sys.argv[2]

done = set()
if os.path.exists(out_path):
    with open(out_path) as f:
        for line in f:
            done.add(line.split("\t", 1)[0])

targets = []
with open(census_path) as f:
    for line in f:
        size, path = line.rstrip("\n").split("\t", 1)
        if size.startswith("STATFAIL"):
            continue
        name = os.path.basename(path)
        ext = os.path.splitext(name)[1].lower()
        if ext in VIDEO_EXTS and not name.startswith("._") and path not in done:
            targets.append((path, size))

print(f"to probe: {len(targets)} (already done: {len(done)})", flush=True)

q = queue.Queue()
for t in targets:
    q.put(t)

lock = threading.Lock()
out = open(out_path, "a")
counter = [0]


def clean(text: str) -> str:
    return text.replace("\t", " ").replace("\n", " | ").strip()[:400]


def worker():
    while True:
        try:
            path, size = q.get_nowait()
        except queue.Empty:
            return
        try:
            proc = subprocess.run(
                ["ffprobe", "-v", "error", "-of", "json",
                 "-show_entries", "format=format_name,duration",
                 "-i", path],
                capture_output=True, text=True, timeout=120,
            )
            if proc.returncode == 0 and proc.stdout:
                info = json.loads(proc.stdout).get("format", {})
                fmt = info.get("format_name", "?")
                dur = info.get("duration", "?")
                err = clean(proc.stderr)
                verdict = "OK" if not err else "OK_WARN"
                row = f"{path}\t{size}\t{verdict}\t{fmt}\t{dur}\t{err}"
            else:
                row = f"{path}\t{size}\tFAIL\t?\t?\t{clean(proc.stderr) or 'ffprobe rc=' + str(proc.returncode)}"
        except subprocess.TimeoutExpired:
            row = f"{path}\t{size}\tTIMEOUT\t?\t?\tffprobe exceeded 120s"
        except Exception as e:  # noqa: BLE001 - record and continue, never abort the sweep
            row = f"{path}\t{size}\tERROR\t?\t?\t{clean(str(e))}"
        with lock:
            out.write(row + "\n")
            out.flush()
            counter[0] += 1
            if counter[0] % 200 == 0:
                print(f"probed {counter[0]}/{len(targets)}", flush=True)


threads = [threading.Thread(target=worker) for _ in range(WORKERS)]
for t in threads:
    t.start()
for t in threads:
    t.join()
out.close()
print(f"done: {counter[0]} probed this run", flush=True)
