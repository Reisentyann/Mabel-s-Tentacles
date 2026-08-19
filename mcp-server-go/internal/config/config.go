package config

import "os"

type Config struct {
	Port    string
	BaseURL string
	DataDir string
}

func Load() *Config {
	return &Config{
		Port:    getenv("MCP_PORT", "8001"),
		BaseURL: getenv("MCP_BASE_URL", "http://localhost:8001"),
		DataDir: getenv("DATA_DIR", "./data"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
