package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Port    string `yaml:"port"`
	BaseURL string `yaml:"base_url"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	File   string `yaml:"file"` // 日志文件路径；空 = 只输出控制台
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

type MCPConfig struct {
	APIKey string `yaml:"api_key"`
}

type APIConfig struct {
	AccessToken     string `yaml:"access_token"`
	RequireAuth     bool   `yaml:"require_auth"`      // true 时 /api/* 需要 JWT；开发阶段设 false
	DownloadBaseURL string `yaml:"download_base_url"` // 对外下载地址前缀，如 http://host:18080（空=不返回下载链接）
}

type SecurityConfig struct {
	SecretKey              string `yaml:"secret_key"`
	Algorithm              string `yaml:"algorithm"`
	AccessTokenExpireMin   int    `yaml:"access_token_expire_minutes"`
	RefreshTokenExpireDays int    `yaml:"refresh_token_expire_days"`
}

type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	DataDir  string         `yaml:"data_dir"`
	Log      LogConfig      `yaml:"log"`
	Database DatabaseConfig `yaml:"database"`
	MCP      MCPConfig      `yaml:"mcp"`
	API      APIConfig      `yaml:"api"`
	Security SecurityConfig `yaml:"security"`
	Admin    AdminConfig    `yaml:"admin"`
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
		},
		DataDir: "./data",
		Log: LogConfig{
			Level:  "info",
			Format: "json",
			File:   "logs/server.log",
		},
		Database: DatabaseConfig{
			Host:     "postgres",
			Port:     "5432",
			User:     "postgres",
			Password: "postgres",
			Name:     "agent_db",
			MaxConns: 10,
		},
		MCP: MCPConfig{},
		API: APIConfig{},
		Security: SecurityConfig{
			SecretKey:              "supersecretkey",
			Algorithm:              "HS256",
			AccessTokenExpireMin:   30,
			RefreshTokenExpireDays: 7,
		},
		Admin: AdminConfig{
			Username: "admin",
			Password: "admin123",
		},
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
		c.DataDir = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		c.Log.Format = v
	}
	if v := os.Getenv("LOG_FILE"); v != "" {
		c.Log.File = v
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.Database.URL = v
	}
	if v := os.Getenv("MCP_API_KEY"); v != "" {
		c.MCP.APIKey = v
	}
	if v := os.Getenv("ACCESS_TOKEN"); v != "" {
		c.API.AccessToken = v
	}
	if v := os.Getenv("DOWNLOAD_BASE_URL"); v != "" {
		c.API.DownloadBaseURL = v
	}
	if v := os.Getenv("SECRET_KEY"); v != "" {
		c.Security.SecretKey = v
	}
	if v := os.Getenv("JWT_ALGORITHM"); v != "" {
		c.Security.Algorithm = v
	}
	if v := os.Getenv("ACCESS_TOKEN_EXPIRE_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Security.AccessTokenExpireMin = n
		}
	}
	if v := os.Getenv("REFRESH_TOKEN_EXPIRE_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Security.RefreshTokenExpireDays = n
		}
	}
	if v := os.Getenv("ADMIN_USERNAME"); v != "" {
		c.Admin.Username = v
	}
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		c.Admin.Password = v
	}
}

func (c *Config) DatabaseDSN() string {
	if c.Database.URL != "" {
		return c.Database.URL
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		c.Database.User, c.Database.Password, c.Database.Host, c.Database.Port, c.Database.Name)
}
