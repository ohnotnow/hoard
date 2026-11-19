package hoard

import (
	"context"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

var testPaths = []string{
	"/mnt/media/TV/Fawlty Towers/Fawlty Towers S01E01.mkv",
	"/mnt/media/TV/Fawlty Towers/Fawlty Towers S01E02.mkv",
	"/mnt/media/TV/Fawlty Towers/Fawlty Towers S01E01.srt",
	"/mnt/media/video/Game Set and Match/GSM01.avi",
	"/mnt/media/radio/Bleak Expectations/Bleak Expectations S02E03.mp3",
}

func TestMigrationsApplyFresh(t *testing.T) {
	s := newTestStore(t)
	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version WHERE id = 1").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if want := migrations[len(migrations)-1].version; version != want {
		t.Errorf("schema version = %d, want %d", version, want)
	}
}

func TestIngestAndHave(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	result, err := s.Ingest(ctx, "ssd", testPaths, nil)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Files != 5 || result.Videos != 3 || result.Audio != 1 || result.Other != 1 {
		t.Errorf("ingest counts = %+v", result)
	}
	if result.Titles != 3 {
		t.Errorf("ingest titles = %d, want 3", result.Titles)
	}

	t.Run("exact match", func(t *testing.T) {
		matches, err := s.Have(ctx, "fawlty towers")
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("matches = %d, want 1", len(matches))
		}
		m := matches[0]
		if m.Title != "Fawlty Towers" || m.Videos != 2 || m.Audio != 0 {
			t.Errorf("match = %+v", m)
		}
		if len(m.Sources) != 1 || m.Sources[0] != "ssd" {
			t.Errorf("sources = %v", m.Sources)
		}
	})

	t.Run("substring match", func(t *testing.T) {
		matches, err := s.Have(ctx, "expectations")
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 1 || matches[0].Title != "Bleak Expectations" {
			t.Errorf("matches = %+v", matches)
		}
	})

	t.Run("all-words match", func(t *testing.T) {
		matches, err := s.Have(ctx, "towers fawlty nonsense")
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("all-words with junk word should miss, got %+v", matches)
		}
		matches, err = s.Have(ctx, "towers fawlty")
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 1 || matches[0].Title != "Fawlty Towers" {
			t.Errorf("matches = %+v", matches)
		}
	})

	t.Run("reverse containment", func(t *testing.T) {
		// Stored key lacks the article; a longer query must still find it.
		matches, err := s.Have(ctx, "the fawlty towers")
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 1 || matches[0].Title != "Fawlty Towers" {
			t.Errorf("matches = %+v", matches)
		}
	})

	t.Run("miss", func(t *testing.T) {
		matches, err := s.Have(ctx, "the omega factor")
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("expected no matches, got %+v", matches)
		}
	})
}

func TestHaveExactMatchKeepsSubstringMatches(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	paths := append(append([]string{}, testPaths...),
		"/mnt/media/Francis Durbridge Presents Volume 1/Melissa, part 1.avi",
		"/mnt/media/video/Melissa1974/Melissa1974-1.avi",
		"/mnt/media/video/Melissa1974/Melissa1974-2.avi",
		"/mnt/media/TV/Inside the Fawlty Towers Hotel/Inside the Fawlty Towers Hotel.mkv",
	)
	if _, err := s.Ingest(ctx, "ssd", paths, nil); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	matches, err := s.Have(ctx, "melissa")
	if err != nil {
		t.Fatalf("have: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %+v, want the exact title and the 1974 one", matches)
	}
	// The exact key leads even though the other title has more files.
	if matches[0].Key != "melissa" || matches[1].Key != "melissa1974" {
		t.Errorf("order = %q, %q; want melissa, melissa1974", matches[0].Key, matches[1].Key)
	}

	t.Run("spaced query finds the unspaced key", func(t *testing.T) {
		// "melissa 1974" is neither exact nor a substring of "melissa1974";
		// only all-words matching finds it, and it must not be hidden by the
		// plain "melissa" key sitting word-bounded inside the query.
		matches, err := s.Have(ctx, "melissa 1974")
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 2 {
			t.Fatalf("matches = %+v, want both productions", matches)
		}
		if matches[0].Key != "melissa1974" || matches[1].Key != "melissa" {
			t.Errorf("order = %q, %q; want melissa1974, melissa", matches[0].Key, matches[1].Key)
		}
	})

	t.Run("a phrase match does not hide reverse containment", func(t *testing.T) {
		// "inside the fawlty towers hotel" contains the whole query, but the
		// article-less "fawlty towers" key must still be returned after it.
		matches, err := s.Have(ctx, "the fawlty towers")
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 2 {
			t.Fatalf("matches = %+v, want both titles", matches)
		}
		if matches[0].Key != "inside the fawlty towers hotel" || matches[1].Key != "fawlty towers" {
			t.Errorf("order = %q, %q", matches[0].Key, matches[1].Key)
		}
	})
}

func TestHavePathsFallback(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	paths := append(append([]string{}, testPaths...),
		"/mnt/media/skorpion/episode one.avi",
		"/mnt/media/skorpion/episode two.avi",
		"/mnt/media/TV/The.Omega.Factor/The.Omega.Factor.S01E01.avi",
	)
	if _, err := s.Ingest(ctx, "ssd", paths, nil); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// Spelled-out episode numbers defeat the canonicaliser: the files are
	// titled "episode one"/"episode two", so the title search misses...
	matches, err := s.Have(ctx, "skorpion")
	if err != nil {
		t.Fatalf("have: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no title matches, got %+v", matches)
	}

	// ...and the path fallback still finds them.
	found, pattern, err := s.HavePaths(ctx, "skorpion")
	if err != nil {
		t.Fatalf("have paths: %v", err)
	}
	if len(found) != 2 {
		t.Errorf("path matches = %v", found)
	}
	if pattern != "skorpion" {
		t.Errorf("pattern = %q", pattern)
	}

	t.Run("separator agnostic", func(t *testing.T) {
		// A spaced query must match dotted filenames.
		found, pattern, err := s.HavePaths(ctx, "the omega factor")
		if err != nil {
			t.Fatalf("have paths: %v", err)
		}
		if len(found) != 1 {
			t.Errorf("path matches = %v", found)
		}
		if want := "the[^a-z0-9]+omega[^a-z0-9]+factor"; pattern != want {
			t.Errorf("pattern = %q, want %q", pattern, want)
		}
	})

	t.Run("miss", func(t *testing.T) {
		found, _, err := s.HavePaths(ctx, "the sweeney")
		if err != nil {
			t.Fatalf("have paths: %v", err)
		}
		if len(found) != 0 {
			t.Errorf("path matches = %v", found)
		}
	})
}

func TestAliases(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.Ingest(ctx, "ssd", testPaths, nil); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	display, err := s.AddAlias(ctx, "Game Set and Match", "GSM")
	if err != nil {
		t.Fatalf("add alias: %v", err)
	}
	if display != "Game Set and Match" {
		t.Errorf("display = %q", display)
	}

	matches, err := s.Have(ctx, "gsm")
	if err != nil {
		t.Fatalf("have via alias: %v", err)
	}
	if len(matches) != 1 || matches[0].Title != "Game Set and Match" {
		t.Errorf("matches = %+v", matches)
	}

	if _, err := s.AddAlias(ctx, "No Such Programme", "nsp"); err == nil {
		t.Error("expected not-found error for unknown title")
	}

	if err := s.RemoveAlias(ctx, "gsm"); err != nil {
		t.Fatalf("remove alias: %v", err)
	}
	if err := s.RemoveAlias(ctx, "gsm"); err == nil {
		t.Error("expected not-found error removing absent alias")
	}
}

func TestSnapshotReplace(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.Ingest(ctx, "ssd", testPaths, nil); err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	result, err := s.Ingest(ctx, "ssd", testPaths[:2], nil)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if result.Files != 2 {
		t.Errorf("files after replace = %d, want 2", result.Files)
	}

	// Game Set and Match vanished from the drive, so its title should be gone.
	matches, err := s.Have(ctx, "game set and match")
	if err != nil {
		t.Fatalf("have: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("orphan title survived snapshot replace: %+v", matches)
	}

	sources, err := s.Sources(ctx)
	if err != nil {
		t.Fatalf("sources: %v", err)
	}
	if len(sources) != 1 || sources[0].Files != 2 {
		t.Errorf("sources = %+v", sources)
	}
}

func TestIngestRefusesEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.Ingest(ctx, "ssd", []string{"", "  "}, nil); err == nil {
		t.Error("expected validation error for empty ingest")
	}
}

func TestGrep(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.Ingest(ctx, "ssd", testPaths, nil); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	paths, err := s.Grep(ctx, `gsm\d+`)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(paths) != 1 {
		t.Errorf("grep matches = %v", paths)
	}

	if _, err := s.Grep(ctx, `([`); err == nil {
		t.Error("expected validation error for bad pattern")
	}
}

func TestTitlesAndStats(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.Ingest(ctx, "ssd", testPaths, nil); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	titles, err := s.Titles(ctx, 2)
	if err != nil {
		t.Fatalf("titles: %v", err)
	}
	if len(titles) != 1 || titles[0].Title != "Fawlty Towers" || titles[0].Items != 2 {
		t.Errorf("titles(min 2) = %+v", titles)
	}

	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Files != 5 || stats.Titles != 3 || stats.Videos != 3 {
		t.Errorf("stats = %+v", stats)
	}
}
