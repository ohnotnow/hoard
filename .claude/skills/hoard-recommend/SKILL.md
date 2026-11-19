---
name: hoard-recommend
description: Use when recommending television, film, or radio to someone with a hoard catalogue — "can you recommend a 1970s eco-collapse drama?", "suggest something like The Sandbaggers", "what should I watch next?". Reads the saved viewer profile for taste, then checks every candidate against the catalogue (aliases included) before uttering it, so nothing already owned is ever recommended as a discovery.
version: 1.0.0
license: MIT
metadata:
  hermes:
    tags: [media, television, recommendations, viewer-profile]
    related_skills: [hoard-init, hoard-viewer-profile]
---

# Hoard Recommend

## Overview

Recommendations informed by two facts nobody's memory holds reliably: what this person actually likes (the saved viewer profile), and what they already own (the hoard catalogue).

The founding cautionary tale: *The Omega Factor* was once confidently recommended to a collector as a great obscure find — while all ten episodes sat in `video/scifi/` on an old drive, sniggering. Every candidate gets checked before it is spoken. No exceptions for "surely they'd remember owning that".

## Step 1: Load the Profile

```bash
hoard config --json    # profile lives at <directory of db_path>/viewer_profile.md
```

Read `viewer_profile.md` from beside the database.

- **Missing** → offer the `hoard-viewer-profile` skill to build one. You can still recommend from the immediate request alone, but say the recommendation is flying without instruments.
- **Stale** → compare the profile header's per-source dates against `hoard sources`. If drives have been re-ingested since the profile was generated, mention it and offer a refresh; a mildly stale profile is usually still useful, so don't block on it.

A specific request ("1970s eco-collapse drama") narrows the field; the profile decides *which* 1970s eco-collapse dramas fit — tone, geography, format, tolerance for surviving-VHS picture quality.

## Step 2: Brainstorm Wide, Privately

Generate more candidates than you intend to present — the ownership check will eat some. Use the profile's evidence clusters, not just its genre labels: someone whose profile says "institutions under moral stress" wants *The Guardians*, not generic dystopia.

Do not show this raw list to the user. Unchecked candidates are exactly the output this skill exists to prevent.

## Step 3: Check Every Candidate

For each candidate, in order of laziness:

```bash
hoard have 'the guardians'            # exact, phrase and all-words matches, best first
hoard grep 'guardian'                 # regex over every raw path
```

Search the forms a filename would actually take: original-language titles (`Rejseholdet`, not just *Unit One*), punctuation-free forms, common abbreviations, and translated titles. `hoard grep` is the safety net — it sees raw paths, so it catches spellings the canonicaliser split oddly.

Classify each candidate:

- **Absent with reasonable confidence** — `have` and `grep` both blank → safe to recommend.
- **Uncertain / possible alias** — a near-miss or odd grep hit → inspect the matching paths before deciding; if it turns out to be the same programme under another name, offer `hoard alias add TITLE ALIAS` so the catalogue learns it permanently.
- **Already present** — do not recommend as new. If it fits the request beautifully, celebrate the rediscovery instead: "you already own this — all ten episodes, on the old drive" is a genuinely good answer.

Exit codes make the check scriptable: `hoard have` returns 0 on found, 1 on ran-fine-but-nothing-matched. When no canonical title matches, `have` falls back to a raw-path search itself and reports "matching paths" (still exit 0) - treat those hits as owned files whose title the canonicaliser mangled, and inspect the paths as for any near-miss.

## Step 4: Present the Shortlist

For each recommendation:

- one or two sentences on *why it fits* — tied to named profile evidence ("your Sandbaggers/Callan seam"), not generic praise;
- its checked status, stated plainly ("not in your catalogue under any spelling I tried");
- any uncertainty labelled honestly ("possible alias overlap with X — I checked the paths and it's a different programme").

Rediscoveries go in their own little section — they are often the best part.

**Do not claim current availability** — streaming, purchase, broadcast, or archive status — without a separate live lookup. The catalogue knows what is owned; it knows nothing about the outside world.

## Pitfalls

1. **Recommendation amnesia.** The Omega Factor problem. Check every candidate, every time, aliases included.
2. **Canonical-table tunnel vision.** `hoard have` blank is not enough when the spelling might differ — `hoard grep` the raw paths too.
3. **Profile literalism.** The profile is evidence-backed inference, not gospel. The user's request in front of you outranks it.
4. **Availability overclaim.** "You'd love it" is this skill's job; "it's on iPlayer" is not, without checking.
5. **Showing the unchecked list.** Brainstorm privately; present only checked results.
6. **Forgetting to teach.** A discovered alias that isn't fed to `hoard alias add` will cost the same detective work next time.
