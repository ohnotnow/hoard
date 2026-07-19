---
name: hoard-init
description: Conversational onboarding for hoard. Use when someone in this repo asks to set up their media catalogue — "could we scan my media box and my local drive?" — and there is no working config yet, or a new source needs adding. Interviews the user source by source, verifies connectivity read-only before writing anything, builds the yaml config, runs the first sync, and ends by offering to promote the bundled skills to ~/.claude/skills and to build a viewer profile.
version: 1.0.0
license: MIT
metadata:
  hermes:
    tags: [media, catalogue, onboarding, config]
    related_skills: [hoard-viewer-profile, hoard-recommend]
---

# Hoard Init

## Overview

Walk a user from "freshly cloned repo" to "working catalogue" in one conversation. The interview does the remembering so the user doesn't have to: hostnames, mount points, and drive names go straight into a config file on their machine — never into this repo.

Everything here is **read-only towards the media**. `hoard` ingests `find` output; it never touches the drives themselves. Say so early — people are rightly wary of tools pointed at their only copy of something.

## Step 1: Find or Build the Binary

```bash
command -v hoard || ls ./hoard
```

Neither present? Offer to build it (`go build -o hoard .` in the repo root, or `go install github.com/ohnotnow/hoard@latest` for a permanent copy on `$PATH`).

## Step 2: Check What Already Exists

```bash
hoard config --json
```

- `config_exists: true` with a populated database → nothing to init. Ask whether they want to add a source or just `hoard sync`.
- `config_exists: true` but sources are failing or incomplete → skip to the interview for the missing pieces only.
- No config → full interview.

## Step 3: The Interview

One source at a time. For each, you need:

1. **A short label** — `mediabox`, `old-drive`. It tags every path in the catalogue.
2. **Where it lives** — an ssh host + directory (`ssh://host/mnt/media`), or a local path (`/Volumes/Archive/video`).
3. **Generic labels** — ask what the drive/volume and its top-level folders are called. Names like `usbdrive01`, `SeagateBackup`, or a root folder literally called `video` must be declared `generic:` so they are never mistaken for programme titles. Users won't know they need this — you have to ask.

Practical interview notes:

- Users say things like "my kodi box" or "the NAS" — get the actual hostname or IP, and the actual directory. If they're unsure, verify in the next step rather than guessing.
- Quote every path you ever run. Media directories are full of spaces, apostrophes, brackets, and Unicode.
- Old drives count. The catalogue's whole purpose is answering "do I already have this?" — a forgotten drive is a false "no".

## Step 4: Verify Before Writing

Test each remote source read-only **before** it goes in the config:

```bash
ssh HOST 'ls "ROOT" | head -5; printf "files: "; find "ROOT" -type f 2>/dev/null | wc -l'
```

BusyBox-safe, small output, proves the host answers, the path exists, and roughly how much is there. For local paths, `ls` + `find | wc -l` likewise. If a host is down, note it and carry on — `hoard sync` tolerates a dead source later.

## Step 5: Write the Config

```bash
hoard config init    # writes a starter file and prints its location
```

Then edit the file at the `config_path` shown by `hoard config --json`, filling in the interviewed sources:

```yaml
sources:
  - label: mediabox
    url: ssh://mediabox/mnt/media
    generic: [mediabox]
  - label: old-drive
    url: /Volumes/Archive/video
    generic: [usbdrive01, video]
```

The config lives outside the repo by design — real hostnames belong on the user's machine, not in version control. Never echo the finished config back into anything that could be committed.

## Step 6: First Sync — and the Payoff

```bash
hoard sync
hoard stats
```

`hoard stats` is the moment the whole thing becomes real — "9,000 files, 400-odd programmes, across two drives" lands well after five minutes of yaml. Present it with a little ceremony. If a source failed, say which and why (host down? path wrong?) and offer to fix or retry just that one: `hoard sync LABEL`.

Sanity-check the canonicalisation while you're here:

```bash
hoard titles --min-items 5 | head -20
```

If a drive name or junk folder appears as a "programme", add it to that source's `generic:` list and re-sync.

## Step 7: Closing Moves

Two offers, in order:

1. **Promote the skills.** This repo's skills only work when Claude runs inside the clone. Copying them makes hoard useful from anywhere:

   ```bash
   cp -R .claude/skills/hoard-init .claude/skills/hoard-viewer-profile .claude/skills/hoard-recommend ~/.claude/skills/
   ```

   Frame it honestly: the catalogue's real life is months from now, in some other directory, wondering "do I already have The Corridor People?" — and that only works if the skills are global.

2. **Offer the profile.** "Would you like me to build a viewer profile from your collection?" — if yes, invoke the `hoard-viewer-profile` skill.

## Pitfalls

1. **Writing config before verifying.** A typo'd hostname discovered during sync wastes the user's patience; discover it during Step 4 instead.
2. **Forgetting generic labels.** The number-one cause of nonsense titles (`usbdrive01`, 47 items). Ask about drive names explicitly.
3. **Real hostnames in repo files.** They go in the user's config only. If you need an example, use `mediabox`.
4. **Flooding the chat.** Connectivity checks are `head -5` and counts, never full listings.
5. **Treating a down host as fatal.** Note it, sync the rest, retry that label later.
