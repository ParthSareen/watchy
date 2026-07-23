package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavePreservesUnknownFieldsAndComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	input := "# keep this comment\nretention_days: 7\nmodel: old\ncustom_setting: yes\n"
	if err := os.WriteFile(path, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{
		ConfigPath:    path,
		RetentionDays: 1,
		Model:         "default",
		Theme:         "green",
		ColorMode:     "auto",
	}
	if err := cfg.loadConfigFile(path); err != nil {
		t.Fatal(err)
	}
	cfg.Theme = "purple"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	for _, expected := range []string{"# keep this comment", "custom_setting: yes", "theme: purple", "model: old"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("saved config missing %q:\n%s", expected, output)
		}
	}
}

func TestLoadRejectsNonMappingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("- not\n- a\n- mapping\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{}
	if err := cfg.loadConfigFile(path); err == nil {
		t.Fatal("expected non-mapping config to fail")
	}
}
