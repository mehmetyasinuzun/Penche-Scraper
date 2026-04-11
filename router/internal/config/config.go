package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root application configuration.
type Config struct {
	Server   ServerConfig       `yaml:"server"`
	Auth     AuthConfig         `yaml:"auth"`
	Storage  StorageConfig      `yaml:"storage"`
	Worker   WorkerConfig       `yaml:"worker"`
	Routes   RoutesConfig       `yaml:"routes"`
	Adapters AdaptersConfig     `yaml:"adapters"`
	Log      LogConfig          `yaml:"log"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type AuthConfig struct {
	SharedSecret string `yaml:"shared_secret"`
}

type StorageConfig struct {
	DSN string `yaml:"dsn"`
}

type WorkerConfig struct {
	PollIntervalMs  int `yaml:"poll_interval_ms"`
	MaxRetries      int `yaml:"max_retries"`
	BaseBackoffMs   int `yaml:"base_backoff_ms"`
	MaxBackoffMs    int `yaml:"max_backoff_ms"`
	ConcurrentJobs  int `yaml:"concurrent_jobs"`
}

// RoutesConfig maps domain patterns to destination names.
type RoutesConfig struct {
	Default     string            `yaml:"default"`
	DomainMap   map[string]string `yaml:"domain_map"`
}

type AdaptersConfig struct {
	Taiga   TaigaConfig   `yaml:"taiga"`
	Webhook WebhookConfig `yaml:"webhook"`
}

type TaigaConfig struct {
	Enabled   bool   `yaml:"enabled"`
	BaseURL   string `yaml:"base_url"`
	AuthToken string `yaml:"auth_token"`
	ProjectID int    `yaml:"project_id"`
	// Status ID to assign new tasks (optional, uses project default if 0)
	StatusID  int    `yaml:"status_id"`
}

type WebhookConfig struct {
	Enabled  bool              `yaml:"enabled"`
	URL      string            `yaml:"url"`
	Method   string            `yaml:"method"`
	Headers  map[string]string `yaml:"headers"`
	TimeoutMs int              `yaml:"timeout_ms"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // "json" | "text"
}

// defaults returns a Config with safe defaults.
func defaults() Config {
	return Config{
		Server: ServerConfig{
			Host: "127.0.0.1",
			Port: 8787,
		},
		Storage: StorageConfig{
			DSN: "penche.db",
		},
		Worker: WorkerConfig{
			PollIntervalMs: 2000,
			MaxRetries:     5,
			BaseBackoffMs:  1000,
			MaxBackoffMs:   60000,
			ConcurrentJobs: 3,
		},
		Routes: RoutesConfig{
			Default: "taiga",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
		Adapters: AdaptersConfig{
			Webhook: WebhookConfig{
				Method:    "POST",
				TimeoutMs: 10000,
			},
		},
	}
}

// Load reads config from a YAML file, then applies environment variable overrides.
func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	if err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config file: %w", err)
		}
	}

	applyEnvOverrides(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return &cfg, nil
}

// applyEnvOverrides applies PENCHE_* env vars over YAML values.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("PENCHE_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("PENCHE_SERVER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = n
		}
	}
	if v := os.Getenv("PENCHE_AUTH_SECRET"); v != "" {
		cfg.Auth.SharedSecret = v
	}
	if v := os.Getenv("PENCHE_STORAGE_DSN"); v != "" {
		cfg.Storage.DSN = v
	}
	if v := os.Getenv("PENCHE_TAIGA_TOKEN"); v != "" {
		cfg.Adapters.Taiga.AuthToken = v
	}
	if v := os.Getenv("PENCHE_TAIGA_PROJECT_ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Adapters.Taiga.ProjectID = n
		}
	}
	if v := os.Getenv("PENCHE_WEBHOOK_URL"); v != "" {
		cfg.Adapters.Webhook.URL = v
	}
	if v := os.Getenv("PENCHE_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
}

func validate(cfg *Config) error {
	if strings.TrimSpace(cfg.Auth.SharedSecret) == "" {
		return fmt.Errorf("auth.shared_secret must not be empty (set PENCHE_AUTH_SECRET env)")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be 1-65535")
	}
	return nil
}

// Addr returns host:port string.
func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
