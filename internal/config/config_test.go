package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fossism/chaind-cli/internal/config"
)

func TestSaveAndLoadConfig(t *testing.T) {
	// Setup a temporary home directory so we don't overwrite the real one
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	expectedUser := "test_dev"
	expectedToken := "ghp_fakeToken12345"

	// 1. Test Save()
	err := config.Save(expectedUser, expectedToken)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify the file was physically created
	configPath := filepath.Join(tmpHome, ".config", "chaind", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at expected path: %s", configPath)
	}

	// 2. Test Load()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Username != expectedUser {
		t.Errorf("Expected username %s, got %s", expectedUser, cfg.Username)
	}

	if cfg.Token != expectedToken {
		t.Errorf("Expected token %s, got %s", expectedToken, cfg.Token)
	}
}
