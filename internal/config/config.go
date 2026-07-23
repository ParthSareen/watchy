package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/parth/watchy/internal/atomicfile"
	"gopkg.in/yaml.v3"
)

type Config struct {
	HomeDir       string `yaml:"-"`
	LogsDir       string `yaml:"-"`
	DBPath        string `yaml:"-"`
	ConfigPath    string `yaml:"-"`
	TicksPath     string `yaml:"-"`
	HistoryPath   string `yaml:"-"`
	RetentionDays int    `yaml:"retention_days"`
	Model         string `yaml:"model"`
	Theme         string `yaml:"theme"`
	ColorMode     string `yaml:"color_mode"`
	document      yaml.Node
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
	ticksPath := filepath.Join(watchyDir, "ticks.json")
	historyPath := filepath.Join(watchyDir, "history.jsonl")

	cfg := &Config{
		HomeDir:       watchyDir,
		LogsDir:       logsDir,
		DBPath:        dbPath,
		ConfigPath:    configPath,
		TicksPath:     ticksPath,
		HistoryPath:   historyPath,
		RetentionDays: 7,
		Model:         "glm-4.7:cloud",
		Theme:         "green",
		ColorMode:     "auto",
	}

	// Load config file if it exists
	if err := cfg.loadConfigFile(configPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("create default config: %w", err)
		}
	}
	if cfg.ColorMode == "" {
		cfg.ColorMode = "auto"
	}
	if cfg.RetentionDays < 1 {
		return nil, fmt.Errorf("retention_days must be at least 1")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model must not be empty")
	}

	return cfg, nil
}

func (c *Config) loadConfigFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config root must be a mapping")
	}
	var values struct {
		RetentionDays *int    `yaml:"retention_days"`
		Model         *string `yaml:"model"`
		Theme         *string `yaml:"theme"`
		ColorMode     *string `yaml:"color_mode"`
	}
	if err := document.Decode(&values); err != nil {
		return err
	}
	if values.RetentionDays != nil {
		c.RetentionDays = *values.RetentionDays
	}
	if values.Model != nil {
		c.Model = *values.Model
	}
	if values.Theme != nil {
		c.Theme = *values.Theme
	}
	if values.ColorMode != nil {
		c.ColorMode = *values.ColorMode
	}
	c.document = document
	return nil
}

// Save persists the current config to disk
func (c *Config) Save() error {
	if len(c.document.Content) == 0 {
		c.document = yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{{
				Kind: yaml.MappingNode,
				Tag:  "!!map",
			}},
		}
	}
	root := c.document.Content[0]
	setScalar(root, "retention_days", "!!int", strconv.Itoa(c.RetentionDays))
	setScalar(root, "model", "!!str", c.Model)
	setScalar(root, "theme", "!!str", c.Theme)
	setScalar(root, "color_mode", "!!str", c.ColorMode)

	var data bytes.Buffer
	encoder := yaml.NewEncoder(&data)
	encoder.SetIndent(2)
	if err := encoder.Encode(&c.document); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("finish config: %w", err)
	}
	return atomicfile.Write(c.ConfigPath, data.Bytes(), 0644)
}

func setScalar(mapping *yaml.Node, key, tag, value string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		mapping.Content[i+1].Kind = yaml.ScalarNode
		mapping.Content[i+1].Tag = tag
		mapping.Content[i+1].Value = value
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value},
	)
}
