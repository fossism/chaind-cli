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
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "chaind")
	os.MkdirAll(configDir, 0755)
	return filepath.Join(configDir, "config.json")
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
