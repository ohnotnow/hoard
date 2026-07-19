---
name: hoard-viewer-profile
description: Use when analysing a personal TV, film, radio, or mixed-media library to infer a viewer profile — "what does my collection say about me?". Prefers the hoard catalogue (binary on $PATH or in the repo) so thousands of messy filenames become instant title data; falls back to a read-only find + Python canonicalisation pipeline when hoard is absent. Shows the profile in conversation, then saves it beside the hoard database where the hoard-recommend skill expects it.
version: 2.0.0
author: Twinky
license: MIT
metadata:
  hermes:
    tags: [media, television, catalogue, viewer-profile]
    related_skills: [hoard-init, hoard-recommend]
---

# Hoard Viewer Profile

## Overview

Turn thousands of untidy media paths into a defensible viewer profile without flooding the conversation or modifying the library. The method separates mechanical reduction from human interpretation:

1. get canonical title data (from hoard, or by building it);
2. inspect recurring and long-tail signals;
3. infer taste using multiple independent evidence clusters;
4. show the profile to the user, then save it for later sessions.

The filename catalogue is evidence of **acquisition or retention**, not proof of viewing or enjoyment. State that limitation explicitly in every profile.

This skill does not make recommendations. That is the `hoard-recommend` skill's job — it reads the profile this skill saves, and checks every candidate against the catalogue before uttering it.

## When to Use

- "What does my TV collection say about me?"
- "Profile my tastes from my media drives."
- "Rummage through this old media drive and tell me who I am."

Do not use this workflow to reorganise, rename, delete, move, copy, or clean media. Those are separate side-effecting tasks requiring explicit permission.

## Safety Contract

The profiling pass is **strictly read-only**:

- Allowed: hoard queries, `ls`, `find`, `du`, `wc`, path export, local analysis of exported path text.
- Forbidden without a separate explicit request: `mv`, `rm`, `cp`, `scp`, torrent removal, metadata rewriting, directory creation on the media source, or library rescans.
- Keep generated inventories and working files outside the source library.
- Quote every root path. Expect spaces, apostrophes, brackets, Unicode, and punctuation.
- Never stream thousands of paths into the conversation. Redirect to files and summarise.

Completion criterion: no source path has been changed and every generated artifact lives outside the media roots.

## Step 1: Resolve the Data Source

Check for hoard first — it usually removes the three most expensive steps entirely.

```bash
command -v hoard || ls ./hoard
hoard config --json
```

- **Binary present, `db_exists: true`** → take the fast path below. Note `db_path` from the output; the profile will be saved next to it.
- **Binary present, no database** → offer to run the `hoard-init` skill (conversational setup and first sync), then return here.
- **No hoard at all** → use the standalone fallback further down. The skill is self-sufficient without hoard; it is just slower.

## Step 2 (fast path): Query the Catalogue

Everything the profile needs is already canonicalised:

```bash
hoard stats            # scale: totals, per-source breakdown, extension counts
hoard sources          # which drives, how fresh each snapshot is
hoard titles --min-items 5   # the head: recurring series/collections
hoard titles           # the full table, long tail included
hoard alias list       # aliases a human has already confirmed
```

Check with the user that `hoard sources` covers every drive they care about — an old drive missing from the catalogue skews the whole profile. If a source looks stale, offer `hoard sync` (it re-reads a `find` listing; still read-only towards the media).

Spot-check a few head titles and a few oddities against raw paths before trusting them:

```bash
hoard grep 'gsm|quatermass'
```

Completion criterion: scale reported, every relevant drive present in `hoard sources`, head and long-tail samples map back sensibly to real paths.

## Step 2 (fallback): Build the Data Yourself

Without hoard, do the mechanical reduction locally. Measure before listing:

```bash
ROOT='/path/to/library'
printf 'files: '; find "$ROOT" -type f 2>/dev/null | wc -l
du -sh "$ROOT" 2>/dev/null
```

Export a compact path inventory (remote example — run `find` remotely, keep the output local):

```bash
ssh host 'find "/remote/media/root" -type f -print 2>/dev/null' > /tmp/media-paths.txt
wc -l /tmp/media-paths.txt
```

Canonicalise with the bundled script (paths are relative to this skill directory):

```bash
python3 scripts/catalogue_media.py \
  --paths-file /tmp/media-paths.txt \
  --output-dir /tmp/media-profile
```

It writes `canonical_titles.tsv`, `summary.json`, and `path_inventory.txt`. Treat folders such as `TV`, `video`, `Film`, `scifi`, and `torrents` as taxonomy, not programme names; add library-specific junk-drawer labels (including drive volume names like `usbdrive01`) with repeated `--generic-dir NAME` options.

The canonicaliser deliberately favours false splits over destructive false merges — `corridor people` and `The Corridor People` may appear separately. Preserve that ambiguity for human review rather than merging aggressively.

## Step 3: Inspect Both the Head and the Long Tail

High episode counts reveal sustained acquisition, but they can swamp the analysis. Weight a title as one preference signal first; use its item count only as supporting intensity. A collection of seventy episodes of one procedural is not seventy times more important than a rare one-off play.

Review:

- leading recurring series;
- medium-sized complete runs;
- one-off plays, documentaries, and films;
- old or low-quality formats retained despite inconvenience;
- audio/radio annexes;
- unusual presenters, writers, directors, anthology strands, and regional imports.

Completion criterion: the evidence set includes recurring favourites and at least several meaningful long-tail items.

## Step 4: Infer a Profile by Evidence Clusters

Build claims only when supported by multiple independent titles or collection behaviours. Useful dimensions:

| Dimension | Signals |
|---|---|
| Genre | crime, food, political thriller, SF, horror, comedy, history |
| Tone | cosy, bleak, satirical, uncanny, procedural, contemplative |
| Geography | UK, Nordic, continental Europe, Australia/NZ, global travel |
| Era | current television, archive decades, surviving or lost productions |
| Format | series, anthology plays, presenter-led factual, radio drama/comedy |
| Narrative interest | institutions, place, partnerships, bureaucracy, social history |
| Collector behaviour | complete runs, VHS tolerance, original-title retention, rare editions |

Prefer interpretations that connect several dimensions, and include disconfirming or balancing evidence where it makes the profile more accurate.

Completion criterion: every major profile claim names representative evidence and avoids reducing the viewer to one large franchise.

## Step 5: State Biases and Confidence

Always distinguish:

- downloaded from watched;
- watched from enjoyed;
- retained from deliberately collected;
- current storage from lifetime viewing;
- file count from unique programme count;
- filename year mentions from a true release-year distribution.

Use qualified language such as "the collection suggests" rather than asserting private psychology as fact. The report must contain at least one explicit limitations paragraph.

## Step 6: Show the Profile, Then Save It

**Show first.** Present the full profile in the conversation — the reading is the payoff. A useful profile contains:

1. scope and scale (drives, file counts, title counts);
2. method in plain language;
3. a memorable profile label;
4. 5–10 evidence-backed taste dimensions;
5. a one-sentence summary;
6. limitations.

**Then save.** Write the same profile to the known location beside the hoard database, so `hoard-recommend` can find it in a later session:

```bash
hoard config --json   # profile path = <directory of db_path>/viewer_profile.md
```

Start the saved file with a metadata header the recommend skill can check for staleness against `hoard sources`:

```markdown
---
generated: 2026-07-19
sources:
  ssd: 2026-07-18T20:11:04Z      # label: last-ingest time at generation
  old-drive: 2026-07-18T21:40:12Z
---
```

In fallback mode (no hoard) there is no known location — ask the user where to save it, and say the header's staleness check will not be available.

### On tone

The profile's **structure** is mandated: label, evidence clusters, limitations, metadata header. Its **voice** is not — match whoever is running the session. Humorous labels are welcome when the user enjoys them; the evidence must remain auditable either way.

## Common Pitfalls

1. **Recursive-output flood.** Never run a huge `ls` or `find` directly into chat.
2. **Episode-count domination.** Count programmes before weighting episode volume.
3. **Generic-folder pollution.** `video`, `TV`, `Film`, and `scifi` are not titles.
4. **Parent-folder blindness.** Files such as `GSM01.avi` need the nearest meaningful ancestor (`Game, Set and Match`).
5. **Missing-drive blindness.** A profile built from one drive when another exists misrepresents the collector. Check `hoard sources` (or ask) before concluding.
6. **Watch-history overclaim.** A filesystem cannot prove that an item was watched or liked.
7. **Year-distribution overclaim.** Release years are inconsistently present and repeated per episode.
8. **Accidental cleanup.** Profiling permission is not permission to rename, copy, or delete.
9. **`.ts` extension ambiguity.** `.ts` is treated as MPEG transport-stream video, so TypeScript code on a mixed drive pollutes video counts. Spot-check when a source mixes code and media.

## Verification Checklist

- [ ] All the user's media sources were included (`hoard sources` or explicit roots).
- [ ] Source access remained read-only.
- [ ] Full recursive output stayed out of the conversation.
- [ ] Generic storage folders were excluded as titles.
- [ ] High-frequency candidates were spot-checked against raw paths.
- [ ] Long-tail archive items were sampled.
- [ ] Profile claims cite multiple examples.
- [ ] Downloaded/retained was not equated with watched/liked.
- [ ] The profile was shown in conversation AND saved with its metadata header.
- [ ] No recommendations were made — that is `hoard-recommend`'s job.
