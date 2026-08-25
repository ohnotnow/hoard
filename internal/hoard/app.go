package hoard

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const usageText = `hoard — what does this media library actually contain?

Usage:
  hoard sync [--config PATH] [LABEL]...
  hoard ingest --source LABEL [--generic NAME]... (FILE|-)
  hoard have [--loose] [--json] QUERY
  hoard alias add TITLE ALIAS | alias rm ALIAS | alias list [--json]
  hoard grep [--json] REGEX
  hoard titles [--min-items N] [--json]
  hoard sources [--json]
  hoard stats [--json]
  hoard config [init] [--json]
  hoard version

All commands accept --db PATH (default: <user-config-dir>/hoard/library.db).

Exit codes: 0 ok / found, 1 no match, 64 usage, 65 validation, 66 not found.`

func Run(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usageText)
		return &CLIError{Code: "usage", ExitCode: 64}
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "sync":
		return runSync(rest)
	case "ingest":
		return runIngest(rest)
	case "have":
		return runHave(rest)
	case "alias":
		return runAlias(rest)
	case "grep":
		return runGrep(rest)
	case "titles":
		return runTitles(rest)
	case "sources":
		return runSources(rest)
	case "stats":
		return runStats(rest)
	case "config":
		return runConfig(rest)
	case "version":
		fmt.Println(Version)
		return nil
	case "help", "-h", "--help":
		fmt.Println(usageText)
		return nil
	default:
		return usageError(fmt.Sprintf("unknown command %q (try: hoard help)", cmd))
	}
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func newFlagSet(name string) (*flag.FlagSet, *string, *bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", "", "database path")
	jsonOut := fs.Bool("json", false, "JSON output")
	return fs, dbPath, jsonOut
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			fmt.Println(usageText)
			return &CLIError{Code: "usage", ExitCode: 0}
		}
		return usageError(fs.Name() + ": " + err.Error())
	}
	return nil
}

func resolveDBPath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", internalError("cannot determine config dir: " + err.Error())
	}
	return filepath.Join(configDir, "hoard", "library.db"), nil
}

func openStore(ctx context.Context, dbPath string) (*Store, error) {
	path, err := resolveDBPath(dbPath)
	if err != nil {
		return nil, err
	}
	return OpenStore(ctx, path)
}

func runConfig(args []string) error {
	initMode := len(args) > 0 && args[0] == "init"
	if initMode {
		args = args[1:]
	}
	fs, dbPath, jsonOut := newFlagSet("config")
	configPath := fs.String("config", "", "config file path")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	cfgPath := *configPath
	if cfgPath == "" {
		var err error
		if cfgPath, err = defaultConfigPath(); err != nil {
			return err
		}
	}
	resolvedDB, err := resolveDBPath(*dbPath)
	if err != nil {
		return err
	}

	if fs.NArg() != 0 {
		return usageError("config: expected no arguments or `init`")
	}

	if initMode {
		if err := configInit(cfgPath); err != nil {
			return err
		}
		fmt.Printf("wrote starter config to %s — edit it, then run `hoard sync`\n", cfgPath)
		return nil
	}

	status := configStatus(cfgPath, resolvedDB)
	if *jsonOut {
		return printJSON(status)
	}
	printConfigStatus(status)
	return nil
}

func runSync(args []string) error {
	fs, dbPath, jsonOut := newFlagSet("sync")
	configPath := fs.String("config", "", "config file path")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	path := *configPath
	if path == "" {
		var err error
		if path, err = defaultConfigPath(); err != nil {
			return err
		}
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}

	ctx := context.Background()
	store, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	results, err := syncSources(ctx, store, cfg, fs.Args())
	if err != nil {
		return err
	}

	failed := 0
	if *jsonOut {
		if err := printJSON(map[string]any{"sources": results}); err != nil {
			return err
		}
		for _, r := range results {
			if r.Error != "" {
				failed++
			}
		}
	} else {
		for _, r := range results {
			if r.Error != "" {
				failed++
				fmt.Printf("%s: FAILED: %s\n", r.Label, r.Error)
				continue
			}
			in := r.Ingest
			fmt.Printf("%s: %d files (%d video, %d audio, %d other), %d titles\n",
				r.Label, in.Files, in.Videos, in.Audio, in.Other, in.Titles)
		}
	}
	if failed > 0 {
		return &CLIError{Code: "sync", Message: fmt.Sprintf("%d of %d sources failed", failed, len(results)), ExitCode: 1}
	}
	return nil
}

func runIngest(args []string) error {
	fs, dbPath, jsonOut := newFlagSet("ingest")
	source := fs.String("source", "", "source label (drive identity), required")
	var generic stringList
	fs.Var(&generic, "generic", "extra generic folder/label name; repeatable")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *source == "" {
		return usageError("ingest: --source is required")
	}
	if fs.NArg() != 1 {
		return usageError("ingest: exactly one paths file (or - for stdin) required")
	}

	paths, err := readPaths(fs.Arg(0))
	if err != nil {
		return err
	}

	ctx := context.Background()
	store, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	result, err := store.Ingest(ctx, *source, paths, generic)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(result)
	}
	fmt.Printf("ingested %s: %d files (%d video, %d audio, %d other), %d titles\n",
		result.Source, result.Files, result.Videos, result.Audio, result.Other, result.Titles)
	return nil
}

func readPaths(arg string) ([]string, error) {
	var reader io.Reader
	if arg == "-" {
		reader = os.Stdin
	} else {
		f, err := os.Open(arg)
		if err != nil {
			return nil, validationError(err.Error())
		}
		defer f.Close()
		reader = f
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var paths []string
	for scanner.Scan() {
		paths = append(paths, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, internalError("reading paths: " + err.Error())
	}
	return paths, nil
}

func runHave(args []string) error {
	fs, dbPath, jsonOut := newFlagSet("have")
	loose := fs.Bool("loose", false, "fall back to all-words matching")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageError(`have: exactly one query required (quote it: hoard have "the omega factor")`)
	}
	query := fs.Arg(0)

	ctx := context.Background()
	store, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	matches, err := store.Have(ctx, query, *loose)
	if err != nil {
		return err
	}
	pathMatches, pattern := []string{}, ""
	if len(matches) == 0 {
		pathMatches, pattern, err = store.HavePaths(ctx, query)
		if err != nil {
			return err
		}
		if pathMatches == nil {
			pathMatches = []string{}
		}
	}
	if *jsonOut {
		out := map[string]any{"query": query, "matches": matches}
		if len(matches) == 0 {
			out["path_matches"] = pathMatches
		}
		if err := printJSON(out); err != nil {
			return err
		}
	} else if len(matches) > 0 {
		printMatches(matches)
	} else if len(pathMatches) > 0 {
		printPathMatches(query, pattern, pathMatches)
	} else {
		fmt.Printf("no matches for %q\n", query)
	}
	if len(matches) == 0 && len(pathMatches) == 0 {
		return noMatch()
	}
	return nil
}

func runAlias(args []string) error {
	if len(args) == 0 {
		return usageError("alias: expected add, rm, or list")
	}
	sub, rest := args[0], args[1:]
	fs, dbPath, jsonOut := newFlagSet("alias " + sub)
	if err := parseFlags(fs, rest); err != nil {
		return err
	}

	ctx := context.Background()
	store, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	switch sub {
	case "add":
		if fs.NArg() != 2 {
			return usageError(`alias add: expected TITLE ALIAS (e.g. hoard alias add "Game Set and Match" gsm)`)
		}
		display, err := store.AddAlias(ctx, fs.Arg(0), fs.Arg(1))
		if err != nil {
			return err
		}
		fmt.Printf("alias %q -> %s\n", fs.Arg(1), display)
		return nil
	case "rm":
		if fs.NArg() != 1 {
			return usageError("alias rm: expected ALIAS")
		}
		if err := store.RemoveAlias(ctx, fs.Arg(0)); err != nil {
			return err
		}
		fmt.Printf("removed alias %q\n", fs.Arg(0))
		return nil
	case "list":
		aliases, err := store.ListAliases(ctx)
		if err != nil {
			return err
		}
		if *jsonOut {
			return printJSON(map[string]any{"aliases": aliases})
		}
		printAliases(aliases)
		return nil
	default:
		return usageError(fmt.Sprintf("alias: unknown subcommand %q", sub))
	}
}

func runGrep(args []string) error {
	fs, dbPath, jsonOut := newFlagSet("grep")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageError("grep: exactly one pattern required")
	}
	pattern := fs.Arg(0)

	ctx := context.Background()
	store, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	paths, err := store.Grep(ctx, pattern)
	if err != nil {
		return err
	}
	if *jsonOut {
		if err := printJSON(map[string]any{"pattern": pattern, "paths": paths}); err != nil {
			return err
		}
	} else {
		for _, p := range paths {
			fmt.Println(p)
		}
	}
	if len(paths) == 0 {
		return noMatch()
	}
	return nil
}

func runTitles(args []string) error {
	fs, dbPath, jsonOut := newFlagSet("titles")
	minItems := fs.Int("min-items", 1, "only titles with at least this many files")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	ctx := context.Background()
	store, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	titles, err := store.Titles(ctx, *minItems)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]any{"titles": titles})
	}
	printTitles(titles)
	return nil
}

func runSources(args []string) error {
	fs, dbPath, jsonOut := newFlagSet("sources")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	ctx := context.Background()
	store, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	sources, err := store.Sources(ctx)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(map[string]any{"sources": sources})
	}
	printSources(sources)
	return nil
}

func runStats(args []string) error {
	fs, dbPath, jsonOut := newFlagSet("stats")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	ctx := context.Background()
	store, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	stats, err := store.Stats(ctx)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(stats)
	}
	printStats(stats)
	return nil
}
