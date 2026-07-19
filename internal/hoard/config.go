package hoard

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed config.example.yaml
var exampleConfig string

type SourceConfig struct {
	Label   string   `yaml:"label"`
	URL     string   `yaml:"url"`
	Generic []string `yaml:"generic"`
}

type Config struct {
	Sources []SourceConfig `yaml:"sources"`
}

func defaultConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", internalError("cannot determine config dir: " + err.Error())
	}
	return filepath.Join(configDir, "hoard", "config.yaml"), nil
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, notFoundError(fmt.Sprintf(
			"no config at %s — run `hoard config init` to create one", path))
	}
	if err != nil {
		return nil, internalError("read config: " + err.Error())
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, validationError("parse config: " + err.Error())
	}
	if len(cfg.Sources) == 0 {
		return nil, validationError("config has no sources")
	}

	seen := make(map[string]bool, len(cfg.Sources))
	for i, src := range cfg.Sources {
		if src.Label == "" {
			return nil, validationError(fmt.Sprintf("source %d: label is required", i+1))
		}
		if seen[src.Label] {
			return nil, validationError("duplicate source label: " + src.Label)
		}
		seen[src.Label] = true
		if src.URL == "" {
			return nil, validationError(fmt.Sprintf("source %q: url is required", src.Label))
		}
	}
	return &cfg, nil
}

// configInit writes the embedded starter config to path, refusing to
// clobber an existing file.
func configInit(path string) error {
	if _, err := os.Stat(path); err == nil {
		return validationError("config already exists at " + path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return internalError("create config directory: " + err.Error())
	}
	if err := os.WriteFile(path, []byte(exampleConfig), 0o644); err != nil {
		return internalError("write config: " + err.Error())
	}
	return nil
}

type ConfigStatus struct {
	ConfigPath   string   `json:"config_path"`
	ConfigExists bool     `json:"config_exists"`
	ConfigError  string   `json:"config_error,omitempty"`
	Sources      []string `json:"sources,omitempty"`
	DBPath       string   `json:"db_path"`
	DBExists     bool     `json:"db_exists"`
}

func configStatus(configPath, dbPath string) ConfigStatus {
	status := ConfigStatus{ConfigPath: configPath, DBPath: dbPath}
	if _, err := os.Stat(configPath); err == nil {
		status.ConfigExists = true
		cfg, err := LoadConfig(configPath)
		if err != nil {
			status.ConfigError = err.Error()
		} else {
			for _, src := range cfg.Sources {
				status.Sources = append(status.Sources, src.Label)
			}
		}
	}
	if _, err := os.Stat(dbPath); err == nil {
		status.DBExists = true
	}
	return status
}
