package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fossism/chaind-cli/internal/db"
)

func TestInitDB(t *testing.T) {
	// Setup a temporary home directory so we don't mess up real data
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, ".config"))

	// 1. Initialize the SQLite DB
	database, err := db.InitDB()
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}

	if database == nil {
		t.Fatalf("Expected db instance, got nil")
	}

	// 2. Verify that the SQLite db file is actually created
	dbPath := filepath.Join(tmpHome, ".config", "chaind", "chaind.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("Database file was not created at expected path: %s", dbPath)
	}
}
