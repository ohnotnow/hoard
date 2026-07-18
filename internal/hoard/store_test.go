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
		matches, err := s.Have(ctx, "fawlty towers", false)
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
		matches, err := s.Have(ctx, "expectations", false)
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 1 || matches[0].Title != "Bleak Expectations" {
			t.Errorf("matches = %+v", matches)
		}
	})

	t.Run("loose match", func(t *testing.T) {
		matches, err := s.Have(ctx, "towers fawlty nonsense", true)
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("all-words with junk word should miss, got %+v", matches)
		}
		matches, err = s.Have(ctx, "towers fawlty", true)
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 1 || matches[0].Title != "Fawlty Towers" {
			t.Errorf("matches = %+v", matches)
		}
	})

	t.Run("miss", func(t *testing.T) {
		matches, err := s.Have(ctx, "the omega factor", false)
		if err != nil {
			t.Fatalf("have: %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("expected no matches, got %+v", matches)
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

	matches, err := s.Have(ctx, "gsm", false)
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
	matches, err := s.Have(ctx, "game set and match", false)
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
