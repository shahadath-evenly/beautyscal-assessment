// Package config loads and validates application configuration from the
// environment. Configuration is read once at startup and passed explicitly to
// the components that need it; there is no package-level mutable state.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
)

// Config holds every setting the API process needs to run.
type Config struct {
	// AppEnv is a free-form environment label ("local", "docker", "production").
	AppEnv string
	// HTTPPort is the port the API listens on.
	HTTPPort string
	// DatabaseURL is the PostgreSQL connection string.
	DatabaseURL string
	// LogLevel is one of debug, info, warn, error.
	LogLevel string
	// LogFormat is "json" or "text".
	LogFormat string
	// CORSAllowedOrigins lists browser origins permitted to call the API.
	CORSAllowedOrigins []string

	// ReadHeaderTimeout bounds how long the server waits for request headers.
	ReadHeaderTimeout time.Duration
	// ReadTimeout and WriteTimeout bound a whole request and response.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	// IdleTimeout closes keep-alive connections that go quiet.
	IdleTimeout time.Duration
	// RequestTimeout bounds the handling of a single request.
	RequestTimeout time.Duration
	// MaxBodyBytes caps the size of a request body.
	MaxBodyBytes int64
	// ShutdownTimeout bounds graceful shutdown before connections are closed.
	ShutdownTimeout time.Duration

	// DBMaxConns is the upper bound of the pgx connection pool.
	DBMaxConns int32
	// DBMinConns is the number of connections the pool keeps warm.
	DBMinConns int32
	// DBConnectTimeout bounds the initial connectivity check at startup.
	DBConnectTimeout time.Duration

	// EnvFile is the .env file that was read, empty when none was found.
	EnvFile string
	// EnvInjected counts the variables that file contributed, so startup
	// output can report them.
	EnvInjected int
}

// Load reads configuration from the process environment, applying defaults for
// everything optional. It returns an error when a required value is absent or
// a provided value cannot be parsed, so the process fails fast at startup
// rather than midway through serving traffic.
//
// Before reading the environment it loads a .env file from the working
// directory, if one exists, so `go run ./cmd/api` behaves the same as running
// through make. Real environment variables take precedence over the file,
// which keeps Docker Compose and shell overrides authoritative. Point
// ENV_FILE at another path to use a different file.
func Load() (Config, error) {
	envFile := DefaultEnvFile
	if v, ok := os.LookupEnv("ENV_FILE"); ok && strings.TrimSpace(v) != "" {
		envFile = strings.TrimSpace(v)
	}
	injected, found, err := loadEnvFile(envFile)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:             env("APP_ENV", "local"),
		HTTPPort:           env("HTTP_PORT", "8081"),
		DatabaseURL:        env("DATABASE_URL", ""),
		LogLevel:           strings.ToLower(env("LOG_LEVEL", "info")),
		LogFormat:          strings.ToLower(env("LOG_FORMAT", "text")),
		CORSAllowedOrigins: splitAndTrim(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
	}

	if found {
		cfg.EnvFile = envFile
		cfg.EnvInjected = injected
	}

	if cfg.ReadHeaderTimeout, err = envDuration("READ_HEADER_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.RequestTimeout, err = envDuration("REQUEST_TIMEOUT", 15*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ReadTimeout, err = envDuration("READ_TIMEOUT", 30*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.WriteTimeout, err = envDuration("WRITE_TIMEOUT", 30*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.IdleTimeout, err = envDuration("IDLE_TIMEOUT", 60*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.MaxBodyBytes, err = envInt64("MAX_BODY_BYTES", httpx.DefaultMaxBodyBytes); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = envDuration("SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.DBConnectTimeout, err = envDuration("DB_CONNECT_TIMEOUT", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxConns, err = envInt32("DB_MAX_CONNS", 10); err != nil {
		return Config{}, err
	}
	if cfg.DBMinConns, err = envInt32("DB_MIN_CONNS", 1); err != nil {
		return Config{}, err
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("config: DATABASE_URL is required")
	}
	if strings.TrimSpace(c.HTTPPort) == "" {
		return errors.New("config: HTTP_PORT must not be empty")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: LOG_LEVEL %q must be one of debug, info, warn, error", c.LogLevel)
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		return fmt.Errorf("config: LOG_FORMAT %q must be one of json, text", c.LogFormat)
	}
	if c.DBMaxConns < 1 {
		return errors.New("config: DB_MAX_CONNS must be at least 1")
	}
	if c.DBMinConns < 0 || c.DBMinConns > c.DBMaxConns {
		return errors.New("config: DB_MIN_CONNS must be between 0 and DB_MAX_CONNS")
	}
	return nil
}

// Addr is the TCP address the HTTP server binds to.
func (c Config) Addr() string { return ":" + c.HTTPPort }

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("config: %s must be a duration such as 10s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("config: %s must be positive", key)
	}
	return d, nil
}

func envInt64(key string, fallback int64) (int64, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer: %w", key, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("config: %s must be positive", key)
	}
	return n, nil
}

func envInt32(key string, fallback int32) (int32, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer: %w", key, err)
	}
	return int32(n), nil
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
