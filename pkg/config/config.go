package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type App struct {
	Env string `toml:"env"` // dev | prod
}

type Log struct {
	Level string `toml:"level"`
}

type Server struct {
	Addr string `toml:"addr"`
}

type DB struct {
	DSN string `toml:"dsn"`
}

type Upstream struct {
	BaseURL string `toml:"base_url"`
}

type AI struct {
	BaseURL        string `toml:"base_url"`
	APIKey         string `toml:"api_key"`
	Model          string `toml:"model"`
	RPM            int    `toml:"rpm"`
	RequestTimeout string `toml:"request_timeout"`

	// RequestTimeoutDur is RequestTimeout parsed at load (see Init). time.Duration
	// has no TextUnmarshaler, so it cannot be decoded from TOML directly.
	RequestTimeoutDur time.Duration `toml:"-"`
}

type Gateway struct {
	WaitTimeout string `toml:"wait_timeout"`

	// WaitTimeoutDur is WaitTimeout parsed at load (see Init).
	WaitTimeoutDur time.Duration `toml:"-"`
}

type HandlerConfig struct {
	Enabled bool   `toml:"enabled"`
	Prompt  string `toml:"prompt"`
}

type Config struct {
	App      App                      `toml:"app"`
	Log      Log                      `toml:"log"`
	Server   Server                   `toml:"server"`
	DB       DB                       `toml:"db"`
	Upstream Upstream                 `toml:"upstream"`
	AI       AI                       `toml:"ai"`
	Gateway  Gateway                  `toml:"gateway"`
	Handlers map[string]HandlerConfig `toml:"handlers"`
}

// C is the loaded global config.
var C *Config

func defaultConfig() Config {
	return Config{
		App:     App{Env: "dev"},
		Log:     Log{Level: "info"},
		Server:  Server{Addr: ":8080"},
		AI:      AI{BaseURL: "https://api.openai.com/v1", RPM: 10000, RequestTimeout: "30s"},
		Gateway: Gateway{WaitTimeout: "10s"},
	}
}

// Init loads the TOML config at configPath into C. If the file is missing, a
// default config is written there first.
func Init(configPath string) error {
	if configPath == "" {
		configPath = "config.toml"
	}

	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		data, err := toml.Marshal(defaultConfig())
		if err != nil {
			return fmt.Errorf("marshal default config: %w", err)
		}
		if dir := filepath.Dir(configPath); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create config dir %s: %w", dir, err)
			}
		}
		if err := os.WriteFile(configPath, data, 0o644); err != nil {
			return fmt.Errorf("write default config %s: %w", configPath, err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config %s: %w", configPath, err)
	}

	cfg := defaultConfig()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config %s: %w", configPath, err)
	}

	// time.Duration has no TextUnmarshaler, so the duration strings are parsed
	// here, once, and a bad value fails startup fast.
	if cfg.AI.RequestTimeoutDur, err = time.ParseDuration(cfg.AI.RequestTimeout); err != nil {
		return fmt.Errorf("parse ai.request_timeout %q: %w", cfg.AI.RequestTimeout, err)
	}
	if cfg.Gateway.WaitTimeoutDur, err = time.ParseDuration(cfg.Gateway.WaitTimeout); err != nil {
		return fmt.Errorf("parse gateway.wait_timeout %q: %w", cfg.Gateway.WaitTimeout, err)
	}

	C = &cfg
	return nil
}
