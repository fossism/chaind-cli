package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

func getConfigFile() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	chaindConfigDir := filepath.Join(configDir, "chaind")
	os.MkdirAll(chaindConfigDir, 0755)
	return filepath.Join(chaindConfigDir, "config.json")
}

func Save(username, token string) error {
	cfg := Config{Username: username, Token: token}
	data, _ := json.Marshal(cfg)
	return os.WriteFile(getConfigFile(), data, 0600)
}

func Load() (*Config, error) {
	data, err := os.ReadFile(getConfigFile())
	if err != nil {
		return nil, err
	}
	var cfg Config
	err = json.Unmarshal(data, &cfg)
	return &cfg, err
}
