package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xincode-ai/xin-code/internal/auth"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "auth", "credentials.json")

	err := auth.SaveCredentials(credPath, "sk-test-key-12345")
	if err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

	key := auth.ReadAPIKeyFromFile(credPath)
	if key != "sk-test-key-12345" {
		t.Errorf("expected sk-test-key-12345, got %q", key)
	}
}

func TestSaveCredentialsCreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	credPath := filepath.Join(tmpDir, "nested", "deep", "credentials.json")

	err := auth.SaveCredentials(credPath, "test-key")
	if err != nil {
		t.Fatalf("SaveCredentials should create parent dirs: %v", err)
	}

	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		t.Error("credentials file was not created")
	}
}

func TestSaveGlobalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	err := SaveGlobalSettings(settingsPath, map[string]string{
		"provider": "anthropic",
		"model":    "claude-sonnet-4-6-20250514",
	})
	if err != nil {
		t.Fatalf("SaveGlobalSettings failed: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "anthropic") {
		t.Errorf("settings should contain provider, got: %s", content)
	}
	if !strings.Contains(content, "claude-sonnet") {
		t.Errorf("settings should contain model, got: %s", content)
	}
}
