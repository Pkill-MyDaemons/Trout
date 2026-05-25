package main

import (
	"encoding/json"
	"os"
)

var configFile = dataPath("config.json")

type DaemonMode string

const (
	DaemonModeNightly    DaemonMode = "nightly"
	DaemonModeResponsive DaemonMode = "responsive"
)

var providers = []string{"claude", "gemini", "groq", "local"}

var defaultModels = map[string]string{
	"claude": "claude-opus-4-7",
	"gemini": "gemini-2.5-pro",
	"groq":   "llama-3.3-70b-versatile",
	"local":  "llama3",
}

type Config struct {
	DaemonMode  DaemonMode `json:"daemon_mode"`
	NightlyTime string     `json:"nightly_time"`
	Provider    string     `json:"provider"`
	Model       string     `json:"model"`
	APIKey      string     `json:"api_key"`
	WorkDir     string     `json:"work_dir"`
	LocalURL    string     `json:"local_url"` // for Ollama etc.
}

func defaultConfig() *Config {
	return &Config{
		DaemonMode:  DaemonModeNightly,
		NightlyTime: "23:00",
		Provider:    "claude",
		Model:       defaultModels["claude"],
		WorkDir:     defaultWorkspace(),
		LocalURL:    "http://localhost:11434",
	}
}

func loadConfig() (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, err
	}
	cfg := defaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func saveConfig(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0600)
}
