package hoard

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/mnt/media":          "'/mnt/media'",
		"/mnt/bob's drive":    `'/mnt/bob'\''s drive'`,
		"/mnt/with space/dir": "'/mnt/with space/dir'",
	}
	for input, want := range cases {
		if got := shellQuote(input); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestFetchPathsUnsupportedScheme(t *testing.T) {
	if _, err := fetchPaths(SourceConfig{Label: "x", URL: "smb://host/share"}); err == nil {
		t.Error("expected error for smb scheme")
	}
}

func TestWalkLocal(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "TV", "Example Show")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Example Show S01E01.mkv", "Example Show S01E02.mkv"} {
		if err := os.WriteFile(filepath.Join(sub, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	paths, err := walkLocal(dir)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("paths = %v", paths)
	}

	if _, err := walkLocal(filepath.Join(dir, "no-such-dir")); err == nil {
		t.Error("expected error for missing directory")
	}
}

func TestSyncSourcesLocal(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	dir := t.TempDir()
	show := filepath.Join(dir, "video", "Example Show")
	if err := os.MkdirAll(show, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(show, "Example Show S01E01.mkv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{Sources: []SourceConfig{
		{Label: "archive", URL: dir},
		{Label: "broken", URL: filepath.Join(dir, "absent")},
	}}

	results, err := syncSources(ctx, s, cfg, nil)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Error != "" || results[0].Ingest == nil || results[0].Ingest.Files != 1 {
		t.Errorf("archive result = %+v", results[0])
	}
	if results[1].Error == "" {
		t.Errorf("broken source should have failed: %+v", results[1])
	}

	matches, err := s.Have(ctx, "example show", false)
	if err != nil {
		t.Fatalf("have: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("matches = %+v", matches)
	}

	if _, err := syncSources(ctx, s, cfg, []string{"nonsense"}); err == nil {
		t.Error("expected error for unknown label filter")
	}

	filtered, err := syncSources(ctx, s, cfg, []string{"archive"})
	if err != nil {
		t.Fatalf("filtered sync: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Label != "archive" {
		t.Errorf("filtered = %+v", filtered)
	}
}
