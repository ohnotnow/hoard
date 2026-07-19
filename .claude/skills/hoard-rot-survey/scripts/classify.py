#!/usr/bin/env python3
"""Tier 1.5: classify sweep failures by null-byte pattern.

Samples 64 bytes at 0%, 25%, 50%, 75% and end-of-file (seek reads, tiny I/O).
Classes:
  HOLLOW  - every sample null: allocated-but-never-written husk
  PARTIAL - some samples null: partially written / truncated copy
  DATA    - no null samples: real bytes throughout; damage is structural
"""
import os
import sys

sweep_path, out_path = sys.argv[1], sys.argv[2]

rows = []
with open(sweep_path) as f:
    for line in f:
        fields = line.rstrip("\n").split("\t")
        if len(fields) >= 3 and fields[2] in ("FAIL", "TIMEOUT", "ERROR"):
            rows.append((fields[0], int(fields[1])))

with open(out_path, "w") as out:
    for path, size in sorted(rows):
        if size == 0:
            out.write(f"{path}\t0\tEMPTY\t0/0\t\n")
            continue
        offsets = sorted({0, size // 4, size // 2, (3 * size) // 4, max(0, size - 64)})
        samples = []
        try:
            with open(path, "rb") as fh:
                for off in offsets:
                    fh.seek(off)
                    samples.append(fh.read(64))
        except OSError as e:
            out.write(f"{path}\t{size}\tREADFAIL\t?\t{e.errno}\n")
            continue
        null_count = sum(1 for s in samples if not s.strip(b"\x00"))
        if null_count == len(samples):
            cls = "HOLLOW"
        elif null_count == 0:
            cls = "DATA"
        else:
            cls = "PARTIAL"
        head_hex = samples[0][:16].hex()
        out.write(f"{path}\t{size}\t{cls}\t{null_count}/{len(samples)}\t{head_hex}\n")
        print(f"{cls}\t{os.path.basename(path)}")
