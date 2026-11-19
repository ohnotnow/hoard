---
name: hoard-rot-survey
description: Use when asking hard questions of a media drive's health — "how much of this drive is actually gubbed?", "did everything survive the rescue rsync?", "find my corrupt/broken videos". Tiered, strictly read-only survey — metadata census, ffprobe container sweep, null-pattern classification, sampled full-decode — plus cross-catalogue coverage checks via hoard so every casualty is labelled lost or covered-elsewhere. Ships with a false-positive bestiary so healthy files aren't mourned.
version: 1.0.0
license: MIT
metadata:
  hermes:
    tags: [media, integrity, corruption, survey, catalogue]
    related_skills: [hoard-init, hoard-viewer-profile]
---

# Hoard Rot Survey

## Overview

Everyone with a NAS has a drive they're afraid to ask hard questions of. This skill asks them cheaply, in tiers — each tier reads more bytes than the last, so most of the verdict arrives in the first few minutes:

- **Tier 0** — metadata census: sizes and counts, zero media reads, seconds.
- **Tier 1** — `ffprobe` container-parse of every video file: structural verdicts (truncation, missing moov, broken index), ~0.5 s/file over a mount.
- **Tier 1.5** — null-pattern classification of every failure: 64-byte samples at five offsets sort husks from partial copies from structural damage.
- **Tier 2** — stratified sampled full-decode to *estimate* silent corruption instead of decoding terabytes.

Then two follow-ups that turn verdicts into answers: ssh-side byte checks for files the mount protocol can't even see, and coverage checks via hoard — a dead file with a healthy twin on another drive is clutter, not loss.

## Safety Contract

- **Strictly read-only.** Probes and byte-samples only. No renaming (even of the trailing-space directory you'll be itching to fix), no deleting husks, no "quick tidy-up". Report; the owner decides.
- **All artifacts live outside the surveyed drive** — and in a *durable* directory, not an ephemeral scratchpad. A previous survey lost its census that way and had to start over.
- **Ask before heavy I/O.** If the drive hangs off an appliance that streams to a TV, confirm it's idle before Tier 1 onwards. When asking, give the owner a duration guess from the census count — ballpark 30 minutes per ~3,000 video files on wired ethernet with SSD-backed storage; slower over Wi-Fi or spinning rust. Present it as a guess, not a guarantee — "about half an hour, probably" beats both a silent grind and a fake-precise promise.
- Quote every path. Media drives are full of spaces, apostrophes, brackets, and worse.

## Step 0: Ground Conditions

- Find the drive: local mount, SMB/NFS mount, or appliance-attached (check `hoard sources` / `hoard config --json` for what's already catalogued and where it lives).
- Configs can have multiple ssh sources on different hosts — resolve the host for the source you're surveying, never "the first URL in the config", and verify with a cheap `ls` of the expected root before believing any "No such file or directory".
- Heavy tiers run on a **workstation over a mount** — appliances with BusyBox userlands have no ffprobe. ssh access to the appliance is still needed for the files the mount can't see (see bestiary).
- Verify tools: `ffprobe`/`ffmpeg` locally; `find`, `dd`, `od`, `stat` remotely.

## Step 1: Tier 0 Census

```bash
python3 scripts/census.py /path/to/mount /durable/output/census.tsv
```

Seconds, metadata only. Reports file/byte totals, video counts, and zero-byte video files (the first confirmed casualties). Prunes `.Spotlight-V100`, `.Trashes`, `.fseventsd` noise.

While you're here, hunt for protocol-invisible files via ssh — they won't be in the census and must be checked separately:

```bash
ssh HOST "find /drive/root -type d -name '* '; find /drive/root -type f -name '*:*'; find /drive/root -type f -name '* '"
```

## Step 2: Tier 1 Sweep

```bash
python3 scripts/sweep.py /durable/output/census.tsv /durable/output/sweep.tsv
```

ffprobe container-parse of every video file (skips AppleDouble `._*`). Resumable — re-running skips already-probed paths. 3 workers; for scale, a real outing did ~2,800 files in ~30 minutes over SMB on wired gigabit to SSD-backed storage. Run it in the background and do the ssh checks meanwhile. Verdicts: `OK`, `OK_WARN` (parsed, with stderr noise — usually benign), `FAIL`, `TIMEOUT`.

## Step 3: Tier 1.5 Classification

```bash
python3 scripts/classify.py /durable/output/sweep.tsv /durable/output/classify.tsv
```

For every failure, 64-byte seek-reads at 0/25/50/75/100% classify:

- **HOLLOW** — every sample null: allocated but never written. The dying-drive rescue-rsync signature; nothing to salvage.
- **PARTIAL** — mixed: an interrupted copy. Some material may play up to the cliff edge.
- **DATA** — real bytes throughout: the damage is structural (e.g. missing moov); remux/repair may be possible.
- **EMPTY** — zero bytes.

The class mix tells you the drive's story: HOLLOW-dominated means the rescue failed *per-file and cleanly* — files came across whole or not at all — which also predicts low silent corruption in the survivors.

## Step 4: ssh Byte Checks

For protocol-invisible or unstattable files (and any FAIL you want a second opinion on), sample head/middle/tail bytes remotely:

```bash
ssh HOST 'f="/drive/root/odd dir /file.avi"; stat -c %s "$f"; dd if="$f" bs=16 count=1 2>/dev/null | od -An -t x1; tail -c 16 "$f" | od -An -t x1'
```

Magic numbers worth knowing: AVI = `RIFF....AVI `, mp4 = `....ftyp`, mkv = `1a45dfa3`; a healthy AVI tail sits inside its `idx1` index (`00dc`/`01wb` entries). All-null head *and* tail → sample a few middle offsets before declaring it hollow.

**BusyBox rakes, paid for the hard way:** `wc -c < file` *reads the whole file* — use `stat`/`ls -la` for sizes or a five-minute check becomes an hour. `find -name` is case-sensitive — use `-iname`. Pipe complex scripts via `ssh HOST 'sh -s' < script.sh` rather than fighting nested quoting around names with spaces, brackets, and ampersands.

## Step 5: Coverage — Lost, or Just Clutter?

For each casualty, ask hoard whether a healthy copy exists anywhere else:

```bash
hoard grep --json 'the.doll|bat out of hell'   # raw paths, all sources
hoard have 'francis durbridge'
```

Classify: **covered elsewhere** (healthy copy on another source — verify it, then celebrate), **covered on-drive** (e.g. `lost+found` fsck orphans duplicating files that exist properly — check before mourning), or **genuinely lost**.

**Verify coverage claims at episode level.** Loose greps lie helpfully: "Film Noir S01E01 (2022)" is not *The Rules of Film Noir* (2009); *Walking the Himalayas* is not *Himalaya with Michael Palin*; a title-level hit for a series says nothing about the specific dead episode. Match programme *and* episode before declaring anything saved.

## Step 6: Tier 2 Decode Sample

```bash
python3 scripts/decode_sample.py /durable/output/sweep.tsv /durable/output/decode_sample.tsv 12
```

Full `ffmpeg` null-decode of a stratified sample of *passing* files (never FAILs — those are known bad) to estimate silent corruption. Near-I/O-bound over a mount, so sample dozens, not thousands; run big samples overnight.

## Step 7: Report

Write it to the durable output directory, structured as:

1. headline survival % (by file count *and* by bytes);
2. method in plain language;
3. failure anatomy (the HOLLOW/PARTIAL/DATA/EMPTY mix and what it implies);
4. **vindicated, not damaged** — the protocol-invisible files that turned out healthy;
5. casualties with a verified healthy copy elsewhere;
6. genuinely lost, grouped so the owner feels the shape of the loss;
7. re-source recommendations (commercial re-buys vs archive rarities), with any space-reclaim option stated as **owner's decision, not done**;
8. explicit read-only completion statement.

## False-Positive Bestiary

Things that look like damage and aren't — where naive surveys waste their time:

1. **Trailing-space directory names** — invisible over SMB; fine via ssh.
2. **Colons (and other reserved characters) in filenames** — unstattable over SMB; fine via ssh.
3. **`Subtitles/` subdirectories** — look like video-less dirs; the video is in the parent. Use parent-aware checks before declaring orphans.
4. **Unicode NFC/NFD** — ssh and SMB views of the same name differ byte-wise; normalise before diffing path lists.
5. **`lost+found` orphans** — fsck salvage that may duplicate healthy files elsewhere on the same drive.
6. **AppleDouble `._*`, `.Spotlight-V100`, `.Trashes`, `.DS_Store`** — old-Mac droppings, not media.
7. **`mp3float: Header missing` warnings** on old XviD rips — benign VBR-audio quirk; the file plays.
8. **eyetv/DVB `.mpg` dimension warnings** — broadcast-stream noise, benign.
9. **`.ts` TypeScript files** on mixed drives — counted as video by extension; check the path.

## Completion Checklist

- [ ] Every tier's artifact exists in the durable output directory.
- [ ] Sweep row count matches the census video count (minus protocol-invisible files, which were ssh-checked instead).
- [ ] Every FAIL is classified, and every casualty labelled lost / covered (verified) / duplicate.
- [ ] The drive was not written to, renamed, or "tidied" in any way.
- [ ] The report states survival by count and by bytes, and separates vindicated files from real losses.
