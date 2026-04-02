package main

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Model == "" {
		t.Error("Model should not be empty")
	}
	if cfg.Permission.Mode != "default" {
		t.Errorf("expected 'default', got '%s'", cfg.Permission.Mode)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	os.Setenv("XINCODE_MODEL", "gpt-4o")
	os.Setenv("XINCODE_API_KEY", "test-key")
	defer os.Unsetenv("XINCODE_MODEL")
	defer os.Unsetenv("XINCODE_API_KEY")

	cfg := LoadConfig()
	if cfg.Model != "gpt-4o" {
		t.Errorf("expected 'gpt-4o', got '%s'", cfg.Model)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("expected 'test-key', got '%s'", cfg.APIKey)
	}
}
