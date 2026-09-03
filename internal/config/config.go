package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root TIDE configuration. Env vars override file values.
// Prefix: TIDE_ with nested keys joined by _ (e.g. TIDE_POSTGRES_DSN).
type Config struct {
	Env        string         `yaml:"env"`
	APIPort    int            `yaml:"apiPort"`
	EnginePort int            `yaml:"enginePort"`
	Postgres   PostgresConfig `yaml:"postgres"`
	Redis      RedisConfig    `yaml:"redis"`
	NATS       NATSConfig     `yaml:"nats"`
	OTELEndpoint string       `yaml:"otelEndpoint"`
}

type PostgresConfig struct {
	DSN string `yaml:"dsn"`
}

type RedisConfig struct {
	Addr string `yaml:"addr"`
}

type NATSConfig struct {
	URL string `yaml:"url"`
}

func Defaults() Config {
	return Config{
		Env:        "dev",
		APIPort:    8080,
		EnginePort: 8081,
		Postgres:   PostgresConfig{DSN: "postgres://tide:tide@localhost:5432/tide?sslmode=disable"},
		Redis:      RedisConfig{Addr: "localhost:6379"},
		NATS:       NATSConfig{URL: "nats://localhost:4222"},
	}
}

// Load reads path (if non-empty) then applies TIDE_ env overrides.
// Invalid config fails fast with a clear error — never a silent default.
func Load(path string) (Config, error) {
	cfg := Defaults()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("config: read %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("TIDE_ENV"); v != "" {
		c.Env = v
	}
	if v := os.Getenv("TIDE_API_PORT"); v != "" {
		var p int
		if _, err := fmt.Sscanf(v, "%d", &p); err == nil {
			c.APIPort = p
		}
	}
	if v := os.Getenv("TIDE_ENGINE_PORT"); v != "" {
		var p int
		if _, err := fmt.Sscanf(v, "%d", &p); err == nil {
			c.EnginePort = p
		}
	}
	if v := os.Getenv("TIDE_POSTGRES_DSN"); v != "" {
		c.Postgres.DSN = v
	}
	if v := os.Getenv("TIDE_REDIS_ADDR"); v != "" {
		c.Redis.Addr = v
	}
	if v := os.Getenv("TIDE_NATS_URL"); v != "" {
		c.NATS.URL = v
	}
	if v := os.Getenv("TIDE_OTEL_ENDPOINT"); v != "" {
		c.OTELEndpoint = v
	}
}

func (c Config) Validate() error {
	var Problem []string
	if c.APIPort <= 0 || c.APIPort > 65535 {
		Problem = append(Problem, "apiPort must be 1-65535")
	}
	if c.EnginePort <= 0 || c.EnginePort > 65535 {
		Problem = append(Problem, "enginePort must be 1-65535")
	}
	if c.APIPort == c.EnginePort {
		Problem = append(Problem, "apiPort and enginePort must differ")
	}
	if !strings.HasPrefix(c.Postgres.DSN, "postgres://") && !strings.HasPrefix(c.Postgres.DSN, "postgresql://") {
		Problem = append(Problem, "postgres.dsn must be a postgres:// URL")
	}
	if strings.TrimSpace(c.Redis.Addr) == "" {
		Problem = append(Problem, "redis.addr must not be empty")
	}
	if !strings.HasPrefix(c.NATS.URL, "nats://") {
		Problem = append(Problem, "nats.url must be a nats:// URL")
	}
	switch c.Env {
	case "dev", "test", "prod":
	default:
		Problem = append(Problem, "env must be one of dev|test|prod")
	}
	if len(Problem) > 0 {
		return fmt.Errorf("config: invalid: %s", strings.Join(Problem, "; "))
	}
	return nil
}
