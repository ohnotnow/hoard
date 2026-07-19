#!/usr/bin/env python3
"""Tier 2: stratified sampled full-decode to estimate silent corruption.

Samples OK/OK_WARN files from the sweep (never FAILs - those are known bad),
stratified by container extension, and runs a full null-muxer decode on each.
Read-only; slow; run few.
"""
import os
import random
import subprocess
import sys
from collections import defaultdict

sweep_path, out_path, n = sys.argv[1], sys.argv[2], int(sys.argv[3])
random.seed(42)

by_ext = defaultdict(list)
warns = []
with open(sweep_path) as f:
    for line in f:
        fields = line.rstrip("\n").split("\t")
        path, verdict = fields[0], fields[2]
        if verdict == "OK":
            by_ext[os.path.splitext(path)[1].lower()].append(path)
        elif verdict == "OK_WARN":
            warns.append(path)

picks = []
exts = sorted(by_ext, key=lambda e: -len(by_ext[e]))
per_ext = max(1, (n - 3) // max(1, len(exts[:5])))
for ext in exts[:5]:
    picks.extend(random.sample(by_ext[ext], min(per_ext, len(by_ext[ext]))))
picks.extend(random.sample(warns, min(3, len(warns))))
remaining = [p for ext in exts for p in by_ext[ext] if p not in picks]
random.shuffle(remaining)
picks.extend(remaining[: n - len(picks)])
picks = picks[:n]

with open(out_path, "w") as out:
    for path in picks:
        try:
            proc = subprocess.run(
                ["ffmpeg", "-v", "error", "-i", path, "-f", "null", "-"],
                capture_output=True, text=True, timeout=1800,
            )
            err = proc.stderr.replace("\t", " ").replace("\n", " | ").strip()[:400]
            verdict = "CLEAN" if proc.returncode == 0 and not err else ("DECODE_ERRORS" if proc.returncode == 0 else "DECODE_FAIL")
        except subprocess.TimeoutExpired:
            verdict, err = "TIMEOUT", "decode exceeded 30min"
        out.write(f"{path}\t{verdict}\t{err}\n")
        out.flush()
        print(f"{verdict}\t{os.path.basename(path)}", flush=True)
print("sample done", flush=True)
