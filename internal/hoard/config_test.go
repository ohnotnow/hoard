package hoard

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	path := writeConfig(t, `
sources:
  - label: mediabox
    url: ssh://mediabox/mnt/media
    generic: [mediabox]
  - label: archive
    url: /tmp/archive
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(cfg.Sources))
	}
	if cfg.Sources[0].Label != "mediabox" || cfg.Sources[0].URL != "ssh://mediabox/mnt/media" {
		t.Errorf("first source = %+v", cfg.Sources[0])
	}
	if len(cfg.Sources[0].Generic) != 1 || cfg.Sources[0].Generic[0] != "mediabox" {
		t.Errorf("generic = %v", cfg.Sources[0].Generic)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	cases := map[string]string{
		"missing label":   "sources:\n  - url: /tmp/x\n",
		"missing url":     "sources:\n  - label: x\n",
		"duplicate label": "sources:\n  - label: x\n    url: /a\n  - label: x\n    url: /b\n",
		"no sources":      "sources: []\n",
		"bad yaml":        "sources: [",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(writeConfig(t, content)); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}

	if _, err := LoadConfig(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Error("expected not-found error for missing file")
	}
}

func TestConfigInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	if err := configInit(path); err != nil {
		t.Fatalf("init: %v", err)
	}

	// The embedded starter must itself be a valid config, forever.
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("embedded example does not parse: %v", err)
	}
	if len(cfg.Sources) == 0 {
		t.Error("embedded example has no sources")
	}

	if err := configInit(path); err == nil {
		t.Error("expected refusal to overwrite existing config")
	}
}

func TestConfigStatus(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "library.db")

	status := configStatus(cfgPath, dbPath)
	if status.ConfigExists || status.DBExists {
		t.Errorf("fresh dir status = %+v", status)
	}

	if err := configInit(cfgPath); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := os.WriteFile(dbPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	status = configStatus(cfgPath, dbPath)
	if !status.ConfigExists || !status.DBExists || status.ConfigError != "" || len(status.Sources) == 0 {
		t.Errorf("populated status = %+v", status)
	}

	if err := os.WriteFile(cfgPath, []byte("sources: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	status = configStatus(cfgPath, dbPath)
	if status.ConfigError == "" {
		t.Error("expected config error for bad yaml")
	}
}
