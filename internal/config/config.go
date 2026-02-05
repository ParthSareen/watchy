package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HomeDir       string
	LogsDir       string
	DBPath        string
	ConfigPath    string
	RetentionDays int    `yaml:"retention_days"`
	Model         string `yaml:"model"`
	Theme         string `yaml:"theme"`
}

// New creates a new Config and ensures directories exist
func New() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	watchyDir := filepath.Join(home, ".watchy")
	logsDir := filepath.Join(watchyDir, "logs")
	dbPath := filepath.Join(watchyDir, "watchy.db")

	// Create directories if they don't exist
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	configPath := filepath.Join(watchyDir, "config.yaml")

	cfg := &Config{
		HomeDir:       watchyDir,
		LogsDir:       logsDir,
		DBPath:        dbPath,
		ConfigPath:    configPath,
		RetentionDays: 1,
		Model:         "glm-4.7:cloud",
		Theme:         "green",
	}

	// Load config file if it exists
	if err := cfg.loadConfigFile(configPath); err != nil {
		// Only fail if the file exists but can't be parsed
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
		// Create default config file
		cfg.writeDefaultConfig(configPath)
	}

	return cfg, nil
}

func (c *Config) loadConfigFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, c)
}

func (c *Config) writeDefaultConfig(path string) {
	c.Save()
}

// Save persists the current config to disk
func (c *Config) Save() error {
	data, err := yaml.Marshal(struct {
		RetentionDays int    `yaml:"retention_days"`
		Model         string `yaml:"model"`
		Theme         string `yaml:"theme"`
	}{
		RetentionDays: c.RetentionDays,
		Model:         c.Model,
		Theme:         c.Theme,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(c.ConfigPath, data, 0644)
}
