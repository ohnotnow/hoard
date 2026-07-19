package hoard

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SyncSourceResult struct {
	Label  string        `json:"label"`
	Ingest *IngestResult `json:"ingest,omitempty"`
	Error  string        `json:"error,omitempty"`
}

// syncSources fetches and ingests each configured source, continuing past
// individual failures. labels filters the run when non-empty.
func syncSources(ctx context.Context, store *Store, cfg *Config, labels []string) ([]SyncSourceResult, error) {
	selected := cfg.Sources
	if len(labels) > 0 {
		byLabel := make(map[string]SourceConfig, len(cfg.Sources))
		for _, src := range cfg.Sources {
			byLabel[src.Label] = src
		}
		selected = selected[:0:0]
		for _, label := range labels {
			src, ok := byLabel[label]
			if !ok {
				return nil, validationError("no configured source labelled " + label)
			}
			selected = append(selected, src)
		}
	}

	var results []SyncSourceResult
	for _, src := range selected {
		result := SyncSourceResult{Label: src.Label}
		paths, err := fetchPaths(src)
		if err == nil {
			var ingest IngestResult
			ingest, err = store.Ingest(ctx, src.Label, paths, src.Generic)
			if err == nil {
				result.Ingest = &ingest
			}
		}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return results, nil
}

func fetchPaths(src SourceConfig) ([]string, error) {
	u, err := url.Parse(src.URL)
	if err != nil || u.Scheme == "" {
		return walkLocal(src.URL)
	}
	switch u.Scheme {
	case "ssh":
		return fetchSSH(u)
	case "file":
		return walkLocal(u.Path)
	default:
		return nil, validationError(fmt.Sprintf(
			"unsupported scheme %q (use ssh://host/path or a local path)", u.Scheme))
	}
}

func fetchSSH(u *url.URL) ([]string, error) {
	host := u.Hostname()
	if u.User != nil && u.User.Username() != "" {
		host = u.User.Username() + "@" + host
	}
	args := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10"}
	if port := u.Port(); port != "" {
		args = append(args, "-p", port)
	}
	// `|| true` keeps partial listings usable (find exits non-zero on
	// unreadable subtrees); genuine ssh failures still error at exit 255,
	// and an empty listing is rejected by ingest's empty-refusal guard.
	args = append(args, host, fmt.Sprintf("find %s -type f 2>/dev/null || true", shellQuote(u.Path)))

	cmd := exec.Command("ssh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ssh %s: %s", host, msg)
	}
	return nonBlankLines(stdout.String()), nil
}

func walkLocal(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, validationError("not a directory: " + root)
	}
	var paths []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries, as find 2>/dev/null would
		}
		if d.Type().IsRegular() {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func nonBlankLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
