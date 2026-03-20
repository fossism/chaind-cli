package db

import (
	"os"
	"path/filepath"

	"github.com/fossism/chaind-cli/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB() (*gorm.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configDir := filepath.Join(home, ".config", "chaind")
	os.MkdirAll(configDir, 0755)

	dbPath := filepath.Join(configDir, "chaind.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&models.Task{})
	if err != nil {
		return nil, err
	}
	return db, nil
}
