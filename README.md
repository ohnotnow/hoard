# hoard

A small, fast answer to one question: **"do I actually have a copy of that?"**

`hoard` keeps a local SQLite catalogue of what's on your media drives —
built from plain `find` output, so the drives themselves are never touched.
It canonicalises messy release-style filenames (`the_thick_of_it_2x03.mkv`,
`GSM01.avi`, `Quatermass.1979.Part1.VHSRip.avi`) into programme titles you
can query in an instant, from any terminal, with the drives powered off.

The repo also ships its own Claude Code skills (see [Skills for
Claude](#skills-for-claude)): the skills do the thinking (viewer profiles,
recommendations, "what was that show with that bloke?"), `hoard` supplies
the facts.

## Install

Download a binary from the releases page, or:

```sh
go install github.com/ohnotnow/hoard@latest
```

## Quick start

```sh
# 1. Export a file listing from wherever the media lives (read-only):
ssh mediabox 'find /mnt/ssd -type f' > ssd-paths.txt

# 2. Ingest it under a source label:
hoard ingest --source ssd ssd-paths.txt

# 3. Ask questions:
hoard have 'the omega factor'
hoard have --loose 'omega'
hoard grep 'play.for.today'
hoard titles --min-items 5
```

Re-run steps 1–2 whenever a drive changes: each ingest **replaces** that
source's snapshot, so additions and deletions are both reflected. Pipe
straight through if you prefer: `ssh mediabox 'find /mnt/ssd -type f' | hoard ingest --source ssd -`

### Or configure once and stop remembering things

```sh
hoard config init   # writes a starter config and prints where it put it
hoard config        # shows the config/db paths and current status
```

Edit the file it created — sources are ssh URLs or local paths, plus any
per-source `generic` labels (drive names and junk-drawer folders that must
never be mistaken for programme titles):

```yaml
sources:
  - label: mediabox
    url: ssh://mediabox/mnt/media
    generic: [mediabox]
  - label: local-archive
    url: /Volumes/Archive/video
```

Refreshing your whole catalogue is then:

```sh
hoard sync            # every configured source
hoard sync mediabox   # just one
```

A host being down fails that source and carries on with the rest (exit 1 if
anything failed). The config lives outside any repo on purpose — real
hostnames belong on your machine, not in version control.

## Commands

| Command | Purpose |
|---|---|
| `sync [LABEL]...` | Fetch and ingest every configured source (or just the named ones). `--config PATH` overrides the config location. |
| `ingest --source LABEL (FILE\|-)` | Replace a source's snapshot from a path listing. `--generic NAME` (repeatable) marks extra folder/drive labels as non-titles. |
| `have QUERY` | Look a title up (exact, then substring; `--loose` adds all-words matching). Alias-aware. When no title matches, falls back to a raw-path search so mis-canonicalised files (a folder of `episode two.avi` names) are still found. |
| `alias add TITLE ALIAS` | Teach it that `gsm` means *Game, Set and Match*. Also `alias rm`, `alias list`. |
| `grep REGEX` | Case-insensitive regex over every raw path. |
| `titles` | The canonical title table (`--min-items N` to trim the long tail). |
| `sources` | Drives/snapshots with last-ingest times. |
| `stats` | Totals, per-source breakdown, extension counts. |
| `config` | Show resolved config/db paths and status; `config init` writes a starter config. |
| `version` | Print the version, and check GitHub for a newer release. |
| `self-update` | Download the latest release binary for this platform, verify it against SHA256SUMS, and replace the running executable (`composer self-update` style). `--check` reports without downloading (exit 0 current / 1 newer available / 2 lookup failed); `--yes` skips the prompt. Redirects to `brew upgrade` / `go install` when the binary is package-manager owned. |

All commands take `--db PATH` (default: `<user-config-dir>/hoard/library.db`)
and `--json` for machine-readable output.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success / found |
| 1 | query ran fine but matched nothing (scriptable: `hoard have 'x' && echo owned`) |
| 64 / 65 / 66 | usage / validation / not found |

## Skills for Claude

The repo carries three Claude Code skills in `.claude/skills/`, so a clone
gets the methodology along with the tool:

| Skill | What it does |
|---|---|
| `hoard-init` | Conversational setup: interviews you about your drives and boxes, verifies access read-only, writes the config, runs the first sync. |
| `hoard-viewer-profile` | Builds a viewer profile from the catalogue ("what does my collection say about me?") and saves it beside the database. Works without hoard too, via a bundled Python pipeline. |
| `hoard-recommend` | Profile-aware recommendations that check every candidate against the catalogue first — so nothing you already own is ever "recommended". |
| `hoard-rot-survey` | Tiered, read-only drive-health survey ("how much of this drive is actually gubbed?"): census, ffprobe sweep, null-pattern triage, sampled decode — then hoard-backed coverage checks to tell genuine losses from clutter. |

Project-scoped skills only fire when Claude Code runs inside this clone,
which suits `hoard-init` (you have just cloned it) but not the other two —
their real life is months later, in some unrelated directory, wondering
"do I already have that?". Promote them:

```sh
cp -R .claude/skills/hoard-* ~/.claude/skills/
```

`hoard-init` offers to do this for you as its closing move.

## Notes on canonicalisation

The title cleaner is a faithful Go port of the Python canonicaliser at
`.claude/skills/hoard-viewer-profile/scripts/catalogue_media.py`, pinned by a
golden-fixture test (`internal/hoard/testdata/`) generated from the Python
original — if the two implementations ever drift, the test fails. It deliberately favours false
splits over false merges: `corridor people` and `The Corridor People` may
appear as two titles, and the `alias` command is the intended fix. Note that
`.ts` is treated as video (MPEG transport stream), so TypeScript files on a
mixed drive will be counted as programmes.

## Development

```sh
go build -o hoard .
go test ./...
```

Stdlib + `modernc.org/sqlite` (pure Go) + `golang.org/x/text`; no CGo, so it
cross-compiles cleanly. Releases are built by `.github/workflows/release.yml`
on version tags.

## Licence

MIT
