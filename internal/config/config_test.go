package config

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	// Server defaults
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 8080)
	}
	if cfg.Server.AdminPassword != "changeme" {
		t.Errorf("Server.AdminPassword = %q, want %q", cfg.Server.AdminPassword, "changeme")
	}
	if cfg.Server.SecretKey != "change-this-to-a-random-string" {
		t.Errorf("Server.SecretKey = %q, want %q", cfg.Server.SecretKey, "change-this-to-a-random-string")
	}

	// SMTP defaults
	if cfg.SMTP.Host != "localhost" {
		t.Errorf("SMTP.Host = %q, want %q", cfg.SMTP.Host, "localhost")
	}
	if cfg.SMTP.Port != 25 {
		t.Errorf("SMTP.Port = %d, want %d", cfg.SMTP.Port, 25)
	}
	if cfg.SMTP.From != "noreply@example.com" {
		t.Errorf("SMTP.From = %q, want %q", cfg.SMTP.From, "noreply@example.com")
	}
	if cfg.SMTP.TLS != false {
		t.Errorf("SMTP.TLS = %v, want false", cfg.SMTP.TLS)
	}
	if cfg.SMTP.AuthMethod != "none" {
		t.Errorf("SMTP.AuthMethod = %q, want %q", cfg.SMTP.AuthMethod, "none")
	}

	// Queue defaults
	if cfg.Queue.Workers != 2 {
		t.Errorf("Queue.Workers = %d, want %d", cfg.Queue.Workers, 2)
	}
	if cfg.Queue.MaxRetries != 3 {
		t.Errorf("Queue.MaxRetries = %d, want %d", cfg.Queue.MaxRetries, 3)
	}
	if cfg.Queue.RetryDelay != "30s" {
		t.Errorf("Queue.RetryDelay = %q, want %q", cfg.Queue.RetryDelay, "30s")
	}

	// Database defaults
	if cfg.Database.Path != "./data/webhook.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "./data/webhook.db")
	}
}

func TestLoadFromFile(t *testing.T) {
	yamlContent := `
server:
  host: "127.0.0.1"
  port: 9090
  admin_password: "supersecret"
  secret_key: "my-secret-key"
smtp:
  host: "mail.example.com"
  port: 587
  username: "user@example.com"
  password: "mailpass"
  from: "sender@example.com"
  tls: true
  auth_method: "plain"
queue:
  workers: 4
  max_retries: 5
  retry_delay: "1m"
database:
  path: "/var/lib/webhook/data.db"
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", cfgPath, err)
	}

	// Server
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "127.0.0.1")
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 9090)
	}
	if cfg.Server.AdminPassword != "supersecret" {
		t.Errorf("Server.AdminPassword = %q, want %q", cfg.Server.AdminPassword, "supersecret")
	}
	if cfg.Server.SecretKey != "my-secret-key" {
		t.Errorf("Server.SecretKey = %q, want %q", cfg.Server.SecretKey, "my-secret-key")
	}

	// SMTP
	if cfg.SMTP.Host != "mail.example.com" {
		t.Errorf("SMTP.Host = %q, want %q", cfg.SMTP.Host, "mail.example.com")
	}
	if cfg.SMTP.Port != 587 {
		t.Errorf("SMTP.Port = %d, want %d", cfg.SMTP.Port, 587)
	}
	if cfg.SMTP.Username != "user@example.com" {
		t.Errorf("SMTP.Username = %q, want %q", cfg.SMTP.Username, "user@example.com")
	}
	if cfg.SMTP.Password != "mailpass" {
		t.Errorf("SMTP.Password = %q, want %q", cfg.SMTP.Password, "mailpass")
	}
	if cfg.SMTP.From != "sender@example.com" {
		t.Errorf("SMTP.From = %q, want %q", cfg.SMTP.From, "sender@example.com")
	}
	if cfg.SMTP.TLS != true {
		t.Errorf("SMTP.TLS = %v, want true", cfg.SMTP.TLS)
	}
	if cfg.SMTP.AuthMethod != "plain" {
		t.Errorf("SMTP.AuthMethod = %q, want %q", cfg.SMTP.AuthMethod, "plain")
	}

	// Queue
	if cfg.Queue.Workers != 4 {
		t.Errorf("Queue.Workers = %d, want %d", cfg.Queue.Workers, 4)
	}
	if cfg.Queue.MaxRetries != 5 {
		t.Errorf("Queue.MaxRetries = %d, want %d", cfg.Queue.MaxRetries, 5)
	}
	if cfg.Queue.RetryDelay != "1m" {
		t.Errorf("Queue.RetryDelay = %q, want %q", cfg.Queue.RetryDelay, "1m")
	}
	if cfg.Queue.RetryDuration != time.Minute {
		t.Errorf("Queue.RetryDuration = %v, want %v", cfg.Queue.RetryDuration, time.Minute)
	}

	// Database
	if cfg.Database.Path != "/var/lib/webhook/data.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "/var/lib/webhook/data.db")
	}
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.yaml")
	badContent := `
server:
  port: [[[invalid
  broken: {{{
`
	if err := os.WriteFile(cfgPath, []byte(badContent), 0644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() with invalid YAML should return an error")
	}
}

func TestLoadFromFile_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load() with missing file should return an error")
	}
}

func TestLoadEmptyPath(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") returned error: %v", err)
	}

	// Should match defaults
	defaults := Default()
	if cfg.Server.Port != defaults.Server.Port {
		t.Errorf("Server.Port = %d, want default %d", cfg.Server.Port, defaults.Server.Port)
	}
	if cfg.SMTP.Host != defaults.SMTP.Host {
		t.Errorf("SMTP.Host = %q, want default %q", cfg.SMTP.Host, defaults.SMTP.Host)
	}
	if cfg.Queue.Workers != defaults.Queue.Workers {
		t.Errorf("Queue.Workers = %d, want default %d", cfg.Queue.Workers, defaults.Queue.Workers)
	}
	if cfg.Database.Path != defaults.Database.Path {
		t.Errorf("Database.Path = %q, want default %q", cfg.Database.Path, defaults.Database.Path)
	}
	// RetryDuration should be parsed from default RetryDelay "30s"
	if cfg.Queue.RetryDuration != 30*time.Second {
		t.Errorf("Queue.RetryDuration = %v, want %v", cfg.Queue.RetryDuration, 30*time.Second)
	}
}

func TestEnvOverrides(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		check   func(t *testing.T, cfg *Config)
	}{
		{
			name: "WEBHOOK_SERVER_PORT overrides port",
			envVars: map[string]string{
				"WEBHOOK_SERVER_PORT": "3000",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Server.Port != 3000 {
					t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
				}
			},
		},
		{
			name: "WEBHOOK_SERVER_HOST overrides host",
			envVars: map[string]string{
				"WEBHOOK_SERVER_HOST": "192.168.1.1",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Server.Host != "192.168.1.1" {
					t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "192.168.1.1")
				}
			},
		},
		{
			name: "WEBHOOK_ADMIN_PASSWORD overrides admin_password",
			envVars: map[string]string{
				"WEBHOOK_ADMIN_PASSWORD": "env-password",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Server.AdminPassword != "env-password" {
					t.Errorf("Server.AdminPassword = %q, want %q", cfg.Server.AdminPassword, "env-password")
				}
			},
		},
		{
			name: "WEBHOOK_SECRET_KEY overrides secret_key",
			envVars: map[string]string{
				"WEBHOOK_SECRET_KEY": "env-secret",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Server.SecretKey != "env-secret" {
					t.Errorf("Server.SecretKey = %q, want %q", cfg.Server.SecretKey, "env-secret")
				}
			},
		},
		{
			name: "WEBHOOK_SMTP_HOST overrides smtp host",
			envVars: map[string]string{
				"WEBHOOK_SMTP_HOST": "smtp.override.com",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.SMTP.Host != "smtp.override.com" {
					t.Errorf("SMTP.Host = %q, want %q", cfg.SMTP.Host, "smtp.override.com")
				}
			},
		},
		{
			name: "WEBHOOK_SMTP_PORT overrides smtp port",
			envVars: map[string]string{
				"WEBHOOK_SMTP_PORT": "465",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.SMTP.Port != 465 {
					t.Errorf("SMTP.Port = %d, want 465", cfg.SMTP.Port)
				}
			},
		},
		{
			name: "WEBHOOK_SMTP_USERNAME overrides smtp username",
			envVars: map[string]string{
				"WEBHOOK_SMTP_USERNAME": "env-user",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.SMTP.Username != "env-user" {
					t.Errorf("SMTP.Username = %q, want %q", cfg.SMTP.Username, "env-user")
				}
			},
		},
		{
			name: "WEBHOOK_SMTP_PASSWORD overrides smtp password",
			envVars: map[string]string{
				"WEBHOOK_SMTP_PASSWORD": "env-mail-pass",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.SMTP.Password != "env-mail-pass" {
					t.Errorf("SMTP.Password = %q, want %q", cfg.SMTP.Password, "env-mail-pass")
				}
			},
		},
		{
			name: "WEBHOOK_SMTP_FROM overrides smtp from",
			envVars: map[string]string{
				"WEBHOOK_SMTP_FROM": "env@example.com",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.SMTP.From != "env@example.com" {
					t.Errorf("SMTP.From = %q, want %q", cfg.SMTP.From, "env@example.com")
				}
			},
		},
		{
			name: "WEBHOOK_SMTP_TLS=true enables TLS",
			envVars: map[string]string{
				"WEBHOOK_SMTP_TLS": "true",
			},
			check: func(t *testing.T, cfg *Config) {
				if !cfg.SMTP.TLS {
					t.Error("SMTP.TLS = false, want true when env is \"true\"")
				}
			},
		},
		{
			name: "WEBHOOK_SMTP_TLS=1 enables TLS",
			envVars: map[string]string{
				"WEBHOOK_SMTP_TLS": "1",
			},
			check: func(t *testing.T, cfg *Config) {
				if !cfg.SMTP.TLS {
					t.Error("SMTP.TLS = false, want true when env is \"1\"")
				}
			},
		},
		{
			name: "WEBHOOK_SMTP_TLS=false disables TLS",
			envVars: map[string]string{
				"WEBHOOK_SMTP_TLS": "false",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.SMTP.TLS {
					t.Error("SMTP.TLS = true, want false when env is \"false\"")
				}
			},
		},
		{
			name: "WEBHOOK_SMTP_AUTH_METHOD overrides auth method",
			envVars: map[string]string{
				"WEBHOOK_SMTP_AUTH_METHOD": "login",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.SMTP.AuthMethod != "login" {
					t.Errorf("SMTP.AuthMethod = %q, want %q", cfg.SMTP.AuthMethod, "login")
				}
			},
		},
		{
			name: "WEBHOOK_DB_PATH overrides database path",
			envVars: map[string]string{
				"WEBHOOK_DB_PATH": "/tmp/test.db",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Database.Path != "/tmp/test.db" {
					t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "/tmp/test.db")
				}
			},
		},
		{
			name: "WEBHOOK_QUEUE_WORKERS overrides workers",
			envVars: map[string]string{
				"WEBHOOK_QUEUE_WORKERS": "8",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Queue.Workers != 8 {
					t.Errorf("Queue.Workers = %d, want 8", cfg.Queue.Workers)
				}
			},
		},
		{
			name: "WEBHOOK_QUEUE_MAX_RETRIES overrides max retries",
			envVars: map[string]string{
				"WEBHOOK_QUEUE_MAX_RETRIES": "10",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Queue.MaxRetries != 10 {
					t.Errorf("Queue.MaxRetries = %d, want 10", cfg.Queue.MaxRetries)
				}
			},
		},
		{
			name: "WEBHOOK_QUEUE_RETRY_DELAY overrides retry delay",
			envVars: map[string]string{
				"WEBHOOK_QUEUE_RETRY_DELAY": "2m",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Queue.RetryDelay != "2m" {
					t.Errorf("Queue.RetryDelay = %q, want %q", cfg.Queue.RetryDelay, "2m")
				}
				if cfg.Queue.RetryDuration != 2*time.Minute {
					t.Errorf("Queue.RetryDuration = %v, want %v", cfg.Queue.RetryDuration, 2*time.Minute)
				}
			},
		},
		{
			name: "env overrides file values",
			envVars: map[string]string{
				"WEBHOOK_SERVER_PORT": "4000",
				"WEBHOOK_SMTP_HOST":  "env-smtp.example.com",
				"WEBHOOK_DB_PATH":    "/env/path.db",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Server.Port != 4000 {
					t.Errorf("Server.Port = %d, want 4000", cfg.Server.Port)
				}
				if cfg.SMTP.Host != "env-smtp.example.com" {
					t.Errorf("SMTP.Host = %q, want %q", cfg.SMTP.Host, "env-smtp.example.com")
				}
				if cfg.Database.Path != "/env/path.db" {
					t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "/env/path.db")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			cfg, err := Load("")
			if err != nil {
				t.Fatalf("Load(\"\") returned error: %v", err)
			}
			tt.check(t, cfg)
		})
	}
}

func TestEnvOverrides_WithFile(t *testing.T) {
	yamlContent := `
server:
  port: 9090
smtp:
  host: "file-smtp.example.com"
  from: "file@example.com"
database:
  path: "/file/path.db"
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	t.Setenv("WEBHOOK_SERVER_PORT", "5555")
	t.Setenv("WEBHOOK_SMTP_HOST", "env-smtp.example.com")
	t.Setenv("WEBHOOK_DB_PATH", "/env/override.db")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", cfgPath, err)
	}

	// Env should override file values
	if cfg.Server.Port != 5555 {
		t.Errorf("Server.Port = %d, want 5555 (env override)", cfg.Server.Port)
	}
	if cfg.SMTP.Host != "env-smtp.example.com" {
		t.Errorf("SMTP.Host = %q, want %q (env override)", cfg.SMTP.Host, "env-smtp.example.com")
	}
	if cfg.Database.Path != "/env/override.db" {
		t.Errorf("Database.Path = %q, want %q (env override)", cfg.Database.Path, "/env/override.db")
	}
	// File value not overridden by env should remain
	if cfg.SMTP.From != "file@example.com" {
		t.Errorf("SMTP.From = %q, want %q (from file)", cfg.SMTP.From, "file@example.com")
	}
}

func TestValidation_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"port zero", 0},
		{"port negative", -1},
		{"port too high", 99999},
		{"port 65536", 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlContent := "server:\n  port: " + strconv.Itoa(tt.port) + "\n"
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("writing temp config: %v", err)
			}
			_, err := Load(cfgPath)
			if err == nil {
				t.Errorf("Load() with port %d should return an error", tt.port)
			}
		})
	}
}

func TestValidation_EmptyFrom(t *testing.T) {
	yamlContent := `
smtp:
  from: ""
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() with empty smtp.from should return an error")
	}
}

func TestValidation_InvalidWorkers(t *testing.T) {
	tests := []struct {
		name    string
		workers int
	}{
		{"zero workers", 0},
		{"negative workers", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlContent := "queue:\n  workers: " + strconv.Itoa(tt.workers) + "\n"
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("writing temp config: %v", err)
			}
			_, err := Load(cfgPath)
			if err == nil {
				t.Errorf("Load() with workers=%d should return an error", tt.workers)
			}
		})
	}
}

func TestValidation_InvalidMaxRetries(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
	}{
		{"negative max_retries", -1},
		{"very negative max_retries", -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlContent := "queue:\n  max_retries: " + strconv.Itoa(tt.maxRetries) + "\n"
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("writing temp config: %v", err)
			}
			_, err := Load(cfgPath)
			if err == nil {
				t.Errorf("Load() with max_retries=%d should return an error", tt.maxRetries)
			}
		})
	}
}

func TestValidation_ZeroMaxRetries_IsValid(t *testing.T) {
	// max_retries=0 should be valid (means no retries)
	yamlContent := `
queue:
  max_retries: 0
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() with max_retries=0 should succeed, got error: %v", err)
	}
	if cfg.Queue.MaxRetries != 0 {
		t.Errorf("Queue.MaxRetries = %d, want 0", cfg.Queue.MaxRetries)
	}
}

func TestRetryDelayParsing(t *testing.T) {
	tests := []struct {
		name        string
		delay       string
		wantDur     time.Duration
		wantErr     bool
	}{
		{"30 seconds", "30s", 30 * time.Second, false},
		{"1 minute", "1m", time.Minute, false},
		{"5 minutes", "5m", 5 * time.Minute, false},
		{"1 hour", "1h", time.Hour, false},
		{"complex duration", "1m30s", 90 * time.Second, false},
		{"invalid xyz", "xyz", 0, true},
		{"invalid empty", "", 0, true},
		{"invalid text", "thirty-seconds", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlContent := "queue:\n  retry_delay: \"" + tt.delay + "\"\n"
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("writing temp config: %v", err)
			}

			cfg, err := Load(cfgPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Load() with retry_delay=%q should return an error", tt.delay)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() with retry_delay=%q returned error: %v", tt.delay, err)
			}
			if cfg.Queue.RetryDuration != tt.wantDur {
				t.Errorf("Queue.RetryDuration = %v, want %v", cfg.Queue.RetryDuration, tt.wantDur)
			}
		})
	}
}

func TestLoadPartialYAML_MergesWithDefaults(t *testing.T) {
	// Only override server port; everything else should be defaults
	yamlContent := `
server:
  port: 3000
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", cfgPath, err)
	}

	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
	// Overridden section's other fields should be zeroed by yaml unmarshal
	// but host was in the same section and not specified -- yaml.Unmarshal into
	// an existing struct preserves unset fields, so defaults should remain
	defaults := Default()
	if cfg.SMTP.Host != defaults.SMTP.Host {
		t.Errorf("SMTP.Host = %q, want default %q", cfg.SMTP.Host, defaults.SMTP.Host)
	}
	if cfg.Queue.Workers != defaults.Queue.Workers {
		t.Errorf("Queue.Workers = %d, want default %d", cfg.Queue.Workers, defaults.Queue.Workers)
	}
	if cfg.Database.Path != defaults.Database.Path {
		t.Errorf("Database.Path = %q, want default %q", cfg.Database.Path, defaults.Database.Path)
	}
}

func TestValidation_ValidPortBoundaries(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"port 1 (minimum valid)", 1},
		{"port 65535 (maximum valid)", 65535},
		{"port 8080 (typical)", 8080},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlContent := "server:\n  port: " + strconv.Itoa(tt.port) + "\n"
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("writing temp config: %v", err)
			}
			cfg, err := Load(cfgPath)
			if err != nil {
				t.Fatalf("Load() with port %d should succeed, got error: %v", tt.port, err)
			}
			if cfg.Server.Port != tt.port {
				t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, tt.port)
			}
		})
	}
}
