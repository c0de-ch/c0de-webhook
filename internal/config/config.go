package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	SMTP     SMTPConfig     `yaml:"smtp"`
	Queue    QueueConfig    `yaml:"queue"`
	Database DatabaseConfig `yaml:"database"`
}

type ServerConfig struct {
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	AdminPassword string `yaml:"admin_password"`
	SecretKey     string `yaml:"secret_key"`
}

type SMTPConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	From       string `yaml:"from"`
	TLS            bool   `yaml:"tls"`
	TLSSkipVerify  bool   `yaml:"tls_skip_verify"`
	AuthMethod     string `yaml:"auth_method"` // plain, login, crammd5, none
}

type QueueConfig struct {
	Workers       int           `yaml:"workers"`
	MaxRetries    int           `yaml:"max_retries"`
	RetryDelay    string        `yaml:"retry_delay"`
	RetryDuration time.Duration `yaml:"-"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:          "0.0.0.0",
			Port:          8080,
			AdminPassword: "changeme",
			SecretKey:     "change-this-to-a-random-string",
		},
		SMTP: SMTPConfig{
			Host:       "localhost",
			Port:       25,
			From:       "noreply@example.com",
			TLS:        false,
			AuthMethod: "none",
		},
		Queue: QueueConfig{
			Workers:    2,
			MaxRetries: 3,
			RetryDelay: "30s",
		},
		Database: DatabaseConfig{
			Path: "./data/webhook.db",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	applyEnvOverrides(cfg)

	dur, err := time.ParseDuration(cfg.Queue.RetryDelay)
	if err != nil {
		return nil, fmt.Errorf("parsing retry_delay %q: %w", cfg.Queue.RetryDelay, err)
	}
	cfg.Queue.RetryDuration = dur

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("WEBHOOK_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("WEBHOOK_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("WEBHOOK_ADMIN_PASSWORD"); v != "" {
		cfg.Server.AdminPassword = v
	}
	if v := os.Getenv("WEBHOOK_SECRET_KEY"); v != "" {
		cfg.Server.SecretKey = v
	}
	if v := os.Getenv("WEBHOOK_SMTP_HOST"); v != "" {
		cfg.SMTP.Host = v
	}
	if v := os.Getenv("WEBHOOK_SMTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.SMTP.Port = port
		}
	}
	if v := os.Getenv("WEBHOOK_SMTP_USERNAME"); v != "" {
		cfg.SMTP.Username = v
	}
	if v := os.Getenv("WEBHOOK_SMTP_PASSWORD"); v != "" {
		cfg.SMTP.Password = v
	}
	if v := os.Getenv("WEBHOOK_SMTP_FROM"); v != "" {
		cfg.SMTP.From = v
	}
	if v := os.Getenv("WEBHOOK_SMTP_TLS"); v != "" {
		cfg.SMTP.TLS = v == "true" || v == "1"
	}
	if v := os.Getenv("WEBHOOK_SMTP_AUTH_METHOD"); v != "" {
		cfg.SMTP.AuthMethod = v
	}
	if v := os.Getenv("WEBHOOK_DB_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("WEBHOOK_QUEUE_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Queue.Workers = n
		}
	}
	if v := os.Getenv("WEBHOOK_QUEUE_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Queue.MaxRetries = n
		}
	}
	if v := os.Getenv("WEBHOOK_QUEUE_RETRY_DELAY"); v != "" {
		cfg.Queue.RetryDelay = v
	}
}

func validate(cfg *Config) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", cfg.Server.Port)
	}
	if cfg.SMTP.From == "" {
		return fmt.Errorf("smtp.from is required")
	}
	if cfg.Queue.Workers < 1 {
		return fmt.Errorf("queue.workers must be >= 1")
	}
	if cfg.Queue.MaxRetries < 0 {
		return fmt.Errorf("queue.max_retries must be >= 0")
	}
	return nil
}
