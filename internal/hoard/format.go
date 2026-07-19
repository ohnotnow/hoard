package hoard

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func printMatches(matches []TitleMatch) {
	for i, m := range matches {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s  (video: %d, audio: %d)  sources: %s\n",
			m.Title, m.Videos, m.Audio, strings.Join(m.Sources, ", "))
		for _, sample := range m.Samples {
			fmt.Printf("  %s\n", sample)
		}
	}
}

func printTitles(titles []TitleRow) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ITEMS\tVIDEO\tAUDIO\tSOURCES\tTITLE")
	for _, t := range titles {
		fmt.Fprintf(w, "%d\t%d\t%d\t%d\t%s\n", t.Items, t.Videos, t.Audio, t.Sources, t.Title)
	}
	w.Flush()
}

func printSources(sources []SourceRow) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SOURCE\tFILES\tVIDEO\tAUDIO\tINGESTED")
	for _, s := range sources {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n", s.Label, s.Files, s.Videos, s.Audio, s.IngestedAt)
	}
	w.Flush()
}

func printAliases(aliases []AliasRow) {
	if len(aliases) == 0 {
		fmt.Println("no aliases defined")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ALIAS\tTITLE")
	for _, a := range aliases {
		fmt.Fprintf(w, "%s\t%s\n", a.Alias, a.Title)
	}
	w.Flush()
}

func printConfigStatus(status ConfigStatus) {
	switch {
	case !status.ConfigExists:
		fmt.Printf("config: %s (missing — run `hoard config init` to create it)\n", status.ConfigPath)
	case status.ConfigError != "":
		fmt.Printf("config: %s (INVALID: %s)\n", status.ConfigPath, status.ConfigError)
	default:
		fmt.Printf("config: %s (ok — %d sources: %s)\n",
			status.ConfigPath, len(status.Sources), strings.Join(status.Sources, ", "))
	}
	if status.DBExists {
		fmt.Printf("db:     %s\n", status.DBPath)
	} else {
		fmt.Printf("db:     %s (missing — created on first ingest/sync)\n", status.DBPath)
	}
}

func printStats(stats StatsResult) {
	fmt.Printf("files: %d (video: %d, audio: %d, other: %d)\n",
		stats.Files, stats.Videos, stats.Audio, stats.Other)
	fmt.Printf("titles: %d, aliases: %d\n", stats.Titles, stats.Aliases)
	if len(stats.Sources) > 0 {
		fmt.Println()
		printSources(stats.Sources)
	}
	if len(stats.Extensions) > 0 {
		fmt.Println()
		var parts []string
		for _, ec := range stats.Extensions {
			ext := ec.Ext
			if ext == "" {
				ext = "(none)"
			}
			parts = append(parts, fmt.Sprintf("%s: %d", ext, ec.Count))
		}
		fmt.Printf("extensions: %s\n", strings.Join(parts, ", "))
	}
}
