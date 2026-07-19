package hoard

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func OpenStore(ctx context.Context, path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := runMigrations(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Ingest replaces the named source's file snapshot with the supplied paths.
func (s *Store) Ingest(ctx context.Context, source string, paths []string, extraGeneric []string) (IngestResult, error) {
	result := IngestResult{Source: source}
	generic := defaultGenericSet(extraGeneric)

	seen := make(map[string]bool, len(paths))
	unique := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		unique = append(unique, p)
	}
	if len(unique) == 0 {
		return result, validationError("no paths read; refusing to replace source " + source)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sources (label, ingested_at) VALUES (?, ?)
		ON CONFLICT(label) DO UPDATE SET ingested_at = excluded.ingested_at`,
		source, now); err != nil {
		return result, err
	}
	var sourceID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM sources WHERE label = ?", source).Scan(&sourceID); err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM files WHERE source_id = ?", sourceID); err != nil {
		return result, err
	}

	insertFile, err := tx.PrepareContext(ctx, `
		INSERT INTO files (source_id, path, ext, media_type, title_id)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return result, err
	}
	defer insertFile.Close()

	titleIDs := make(map[string]int64)
	for _, path := range unique {
		_, base := splitDirsAndBase(path)
		_, ext := stemAndExt(base)
		mediaType := mediaTypeForExt(ext)

		var titleID any
		if mediaType != "other" {
			candidate := candidateFromPath(path, generic)
			key := normKey(candidate)
			if key != "" && !genericTitle(candidate, generic) {
				id, ok := titleIDs[key]
				if !ok {
					id, err = upsertTitle(ctx, tx, key, candidate)
					if err != nil {
						return result, err
					}
					titleIDs[key] = id
				}
				titleID = id
			}
		}

		if _, err := insertFile.ExecContext(ctx, sourceID, path, ext, mediaType, titleID); err != nil {
			return result, err
		}
		result.Files++
		switch mediaType {
		case "video":
			result.Videos++
		case "audio":
			result.Audio++
		default:
			result.Other++
		}
	}

	// Drop titles no longer referenced by any file, unless an alias pins them.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM titles WHERE id NOT IN (
			SELECT DISTINCT title_id FROM files WHERE title_id IS NOT NULL
		) AND id NOT IN (SELECT title_id FROM aliases)`); err != nil {
		return result, err
	}

	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT title_id) FROM files
		WHERE source_id = ? AND title_id IS NOT NULL`, sourceID).Scan(&result.Titles); err != nil {
		return result, err
	}

	return result, tx.Commit()
}

func upsertTitle(ctx context.Context, tx *sql.Tx, key, display string) (int64, error) {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO titles (key, display) VALUES (?, ?)
		ON CONFLICT(key) DO NOTHING`, key, display); err != nil {
		return 0, err
	}
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM titles WHERE key = ?", key).Scan(&id)
	return id, err
}

// Have looks a query up against canonical titles and aliases: exact key
// match first, then substring, then (with loose) all-words matching.
func (s *Store) Have(ctx context.Context, query string, loose bool) ([]TitleMatch, error) {
	key := normKey(query)
	if key == "" {
		return nil, validationError("query normalises to nothing: " + query)
	}

	stages := []struct {
		sql  string
		args []any
	}{
		{`SELECT DISTINCT t.id FROM titles t
			LEFT JOIN aliases a ON a.title_id = t.id
			WHERE t.key = ? OR a.alias_key = ?`, []any{key, key}},
		{`SELECT DISTINCT t.id FROM titles t
			LEFT JOIN aliases a ON a.title_id = t.id
			WHERE t.key LIKE '%' || ? || '%' OR a.alias_key LIKE '%' || ? || '%'`, []any{key, key}},
		// Reverse containment: the stored key appears word-bounded inside a
		// longer query ("the omega factor" finds key "omega factor").
		{`SELECT DISTINCT t.id FROM titles t
			LEFT JOIN aliases a ON a.title_id = t.id
			WHERE ' ' || ? || ' ' LIKE '% ' || t.key || ' %'
			   OR ' ' || ? || ' ' LIKE '% ' || a.alias_key || ' %'`, []any{key, key}},
	}
	if loose {
		var conds []string
		var args []any
		for _, word := range strings.Fields(key) {
			conds = append(conds, "t.key LIKE '%' || ? || '%'")
			args = append(args, word)
		}
		stages = append(stages, struct {
			sql  string
			args []any
		}{"SELECT t.id FROM titles t WHERE " + strings.Join(conds, " AND "), args})
	}

	var ids []int64
	for _, stage := range stages {
		rows, err := s.db.QueryContext(ctx, stage.sql, stage.args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		if len(ids) > 0 {
			break
		}
	}

	matches := make([]TitleMatch, 0, len(ids))
	for _, id := range ids {
		match, err := s.titleMatch(ctx, id)
		if err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	sortMatches(matches)
	return matches, nil
}

func (s *Store) titleMatch(ctx context.Context, id int64) (TitleMatch, error) {
	var m TitleMatch
	err := s.db.QueryRowContext(ctx, `
		SELECT t.display, t.key,
			COALESCE(SUM(CASE WHEN f.media_type = 'video' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN f.media_type = 'audio' THEN 1 ELSE 0 END), 0)
		FROM titles t
		LEFT JOIN files f ON f.title_id = t.id
		WHERE t.id = ?
		GROUP BY t.id`, id).Scan(&m.Title, &m.Key, &m.Videos, &m.Audio)
	if err != nil {
		return m, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT s.label FROM files f
		JOIN sources s ON s.id = f.source_id
		WHERE f.title_id = ? ORDER BY s.label`, id)
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return m, err
		}
		m.Sources = append(m.Sources, label)
	}
	if err := rows.Err(); err != nil {
		return m, err
	}

	sampleRows, err := s.db.QueryContext(ctx, `
		SELECT path FROM files WHERE title_id = ? ORDER BY path LIMIT 3`, id)
	if err != nil {
		return m, err
	}
	defer sampleRows.Close()
	for sampleRows.Next() {
		var path string
		if err := sampleRows.Scan(&path); err != nil {
			return m, err
		}
		m.Samples = append(m.Samples, path)
	}
	return m, sampleRows.Err()
}

func sortMatches(matches []TitleMatch) {
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.Videos+a.Audio != b.Videos+b.Audio {
			return a.Videos+a.Audio > b.Videos+b.Audio
		}
		return a.Title < b.Title
	})
}

func (s *Store) AddAlias(ctx context.Context, title, alias string) (string, error) {
	titleKey := normKey(title)
	aliasKey := normKey(alias)
	if aliasKey == "" {
		return "", validationError("alias normalises to nothing: " + alias)
	}

	var id int64
	var display string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, display FROM titles WHERE key = ?", titleKey).Scan(&id, &display)
	if err == sql.ErrNoRows {
		return "", notFoundError(fmt.Sprintf("no canonical title %q — check `hoard titles` for the exact name", title))
	}
	if err != nil {
		return "", err
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO aliases (title_id, alias_key) VALUES (?, ?)
		ON CONFLICT(alias_key) DO UPDATE SET title_id = excluded.title_id`,
		id, aliasKey); err != nil {
		return "", err
	}
	return display, nil
}

func (s *Store) RemoveAlias(ctx context.Context, alias string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM aliases WHERE alias_key = ?", normKey(alias))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFoundError("no such alias: " + alias)
	}
	return nil
}

func (s *Store) ListAliases(ctx context.Context) ([]AliasRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.alias_key, t.display FROM aliases a
		JOIN titles t ON t.id = a.title_id
		ORDER BY a.alias_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AliasRow
	for rows.Next() {
		var row AliasRow
		if err := rows.Scan(&row.Alias, &row.Title); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Grep runs a case-insensitive Go regexp over every stored path.
func (s *Store) Grep(ctx context.Context, pattern string) ([]string, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, validationError("bad pattern: " + err.Error())
	}
	rows, err := s.db.QueryContext(ctx, "SELECT path FROM files ORDER BY path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		if re.MatchString(path) {
			out = append(out, path)
		}
	}
	return out, rows.Err()
}

func (s *Store) Titles(ctx context.Context, minItems int) ([]TitleRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.display, t.key, COUNT(f.id),
			SUM(CASE WHEN f.media_type = 'video' THEN 1 ELSE 0 END),
			SUM(CASE WHEN f.media_type = 'audio' THEN 1 ELSE 0 END),
			COUNT(DISTINCT f.source_id)
		FROM titles t
		JOIN files f ON f.title_id = t.id
		GROUP BY t.id
		HAVING COUNT(f.id) >= ?
		ORDER BY COUNT(f.id) DESC, LOWER(t.display)`, minItems)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TitleRow
	for rows.Next() {
		var row TitleRow
		if err := rows.Scan(&row.Title, &row.Key, &row.Items, &row.Videos, &row.Audio, &row.Sources); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) Sources(ctx context.Context) ([]SourceRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.label, s.ingested_at, COUNT(f.id),
			COALESCE(SUM(CASE WHEN f.media_type = 'video' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN f.media_type = 'audio' THEN 1 ELSE 0 END), 0)
		FROM sources s
		LEFT JOIN files f ON f.source_id = s.id
		GROUP BY s.id
		ORDER BY s.label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SourceRow
	for rows.Next() {
		var row SourceRow
		if err := rows.Scan(&row.Label, &row.IngestedAt, &row.Files, &row.Videos, &row.Audio); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) Stats(ctx context.Context) (StatsResult, error) {
	var stats StatsResult
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN media_type = 'video' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN media_type = 'audio' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN media_type = 'other' THEN 1 ELSE 0 END), 0)
		FROM files`).Scan(&stats.Files, &stats.Videos, &stats.Audio, &stats.Other)
	if err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM titles").Scan(&stats.Titles); err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM aliases").Scan(&stats.Aliases); err != nil {
		return stats, err
	}

	sources, err := s.Sources(ctx)
	if err != nil {
		return stats, err
	}
	stats.Sources = sources

	rows, err := s.db.QueryContext(ctx, `
		SELECT ext, COUNT(*) FROM files
		GROUP BY ext ORDER BY COUNT(*) DESC, ext LIMIT 12`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var ec ExtCount
		if err := rows.Scan(&ec.Ext, &ec.Count); err != nil {
			return stats, err
		}
		stats.Extensions = append(stats.Extensions, ec)
	}
	return stats, rows.Err()
}
