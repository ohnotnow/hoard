# hoard

A small, fast answer to one question: **"do I actually have a copy of that?"**

`hoard` keeps a local SQLite catalogue of what's on your media drives —
built from plain `find` output, so the drives themselves are never touched.
It canonicalises messy release-style filenames (`the_thick_of_it_2x03.mkv`,
`GSM01.avi`, `Quatermass.1979.Part1.VHSRip.avi`) into programme titles you
can query in an instant, from any terminal, with the drives powered off.

It is the companion data source for the `media-library-viewer-profile`
skill: the skill does the thinking (viewer profiles, recommendations,
"what was that show with that bloke?"), `hoard` supplies the facts.

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

## Commands

| Command | Purpose |
|---|---|
| `ingest --source LABEL (FILE\|-)` | Replace a source's snapshot from a path listing. `--generic NAME` (repeatable) marks extra folder/drive labels as non-titles. |
| `have QUERY` | Look a title up (exact, then substring; `--loose` adds all-words matching). Alias-aware. |
| `alias add TITLE ALIAS` | Teach it that `gsm` means *Game, Set and Match*. Also `alias rm`, `alias list`. |
| `grep REGEX` | Case-insensitive regex over every raw path. |
| `titles` | The canonical title table (`--min-items N` to trim the long tail). |
| `sources` | Drives/snapshots with last-ingest times. |
| `stats` | Totals, per-source breakdown, extension counts. |
| `version` | Print the version. |

All commands take `--db PATH` (default: `<user-config-dir>/hoard/library.db`)
and `--json` for machine-readable output.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success / found |
| 1 | query ran fine but matched nothing (scriptable: `hoard have 'x' && echo owned`) |
| 64 / 65 / 66 | usage / validation / not found |

## Notes on canonicalisation

The title cleaner is a faithful Go port of the Python canonicaliser in the
`media-library-viewer-profile` skill, pinned by a golden-fixture test
(`internal/hoard/testdata/`) generated from the Python original — if the two
implementations ever drift, the test fails. It deliberately favours false
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
