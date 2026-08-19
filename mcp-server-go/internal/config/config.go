package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Port    string `yaml:"port"`
	BaseURL string `yaml:"base_url"`
	DataDir string `yaml:"data_dir"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type DatabaseConfig struct {
	URL      string `yaml:"url"`
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
	MaxConns int32  `yaml:"max_conns"`
}

type AuthConfig struct {
	APIKey string `yaml:"api_key"`
}

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Log      LogConfig      `yaml:"log"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
}

func Load() *Config {
	cfg := defaults()
	cfg.loadYAML()
	cfg.loadEnv()
	return cfg
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port:    "8001",
			BaseURL: "http://localhost:8001",
			DataDir: "./data",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
		Database: DatabaseConfig{
			Host:     "postgres",
			Port:     "5432",
			User:     "postgres",
			Password: "postgres",
			Name:     "agent_db",
			MaxConns: 10,
		},
		Auth: AuthConfig{},
	}
}

func (c *Config) loadYAML() {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "config.yml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = yaml.Unmarshal(data, c)
}

func (c *Config) loadEnv() {
	if v := os.Getenv("MCP_PORT"); v != "" {
		c.Server.Port = v
	}
	if v := os.Getenv("MCP_BASE_URL"); v != "" {
		c.Server.BaseURL = v
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		c.Server.DataDir = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		c.Log.Format = v
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.Database.URL = v
	}
	if v := os.Getenv("MCP_API_KEY"); v != "" {
		c.Auth.APIKey = v
	}
}

func (c *Config) DatabaseDSN() string {
	if c.Database.URL != "" {
		return c.Database.URL
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		c.Database.User, c.Database.Password, c.Database.Host, c.Database.Port, c.Database.Name)
}
