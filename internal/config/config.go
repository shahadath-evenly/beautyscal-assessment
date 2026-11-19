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

	// Email holds the buyer-notification settings.
	Email EmailConfig

	// Expiry holds the stale-request expiry job settings.
	Expiry ExpiryConfig

	// EnvFile is the .env file that was read, empty when none was found.
	EnvFile string
	// EnvInjected counts the variables that file contributed, so startup
	// output can report them.
	EnvInjected int
}

// EmailConfig configures the acceptance notification and its worker.
//
// With no BrevoAPIKey the application still queues every notification and
// still runs the worker; the worker logs each message instead of sending it,
// which the assessment accepts and which keeps the whole path exercised
// without credentials.
type EmailConfig struct {
	// Enabled turns the outbox worker on. Events are queued either way, so
	// turning it off parks messages rather than losing them.
	Enabled bool
	// BrevoAPIKey is the transactional API key. It is read from the
	// environment only: it is never logged, printed or stored.
	BrevoAPIKey string
	// BrevoSenderEmail and BrevoSenderName are the From address.
	BrevoSenderEmail string
	BrevoSenderName  string

	// PollInterval is the idle wait between polls.
	PollInterval time.Duration
	// BatchSize is how many events one poll claims.
	BatchSize int32
	// Concurrency bounds sends in flight.
	Concurrency int
	// SendTimeout bounds one provider call.
	SendTimeout time.Duration
	// MaxAttempts bounds the retries of one event.
	MaxAttempts int32
	// RetryBaseDelay is the first retry delay; it doubles up to
	// RetryMaxDelay.
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	// StaleClaimAfter is how long a claimed event may sit unfinished before
	// another worker may reclaim it.
	StaleClaimAfter time.Duration
}

// Configured reports whether a real provider is available.
func (e EmailConfig) Configured() bool {
	return strings.TrimSpace(e.BrevoAPIKey) != "" && strings.TrimSpace(e.BrevoSenderEmail) != ""
}

// ExpiryConfig configures the background job that expires order requests
// left in 'new' for too long.
type ExpiryConfig struct {
	// Enabled turns the job on. Off, nothing expires and requests stay 'new'
	// indefinitely.
	Enabled bool
	// Interval is the gap between sweeps. Hourly is ample for a seven-day
	// window: the job only has to be far more frequent than MaxAge.
	Interval time.Duration
	// MaxAge is how long a request may sit in 'new'. The business rule is
	// seven days; it is configurable so a demo can watch the job work.
	MaxAge time.Duration
	// Timeout bounds one sweep.
	Timeout time.Duration
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

	if cfg.Email, err = loadEmail(); err != nil {
		return Config{}, err
	}

	if cfg.Expiry, err = loadExpiry(); err != nil {
		return Config{}, err
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadExpiry reads the stale-request expiry settings.
func loadExpiry() (ExpiryConfig, error) {
	var (
		e   ExpiryConfig
		err error
	)

	if e.Enabled, err = envBool("EXPIRY_ENABLED", true); err != nil {
		return ExpiryConfig{}, err
	}
	if e.Interval, err = envDuration("EXPIRY_INTERVAL", time.Hour); err != nil {
		return ExpiryConfig{}, err
	}
	if e.MaxAge, err = envDuration("EXPIRY_MAX_AGE", 7*24*time.Hour); err != nil {
		return ExpiryConfig{}, err
	}
	if e.Timeout, err = envDuration("EXPIRY_TIMEOUT", 30*time.Second); err != nil {
		return ExpiryConfig{}, err
	}
	return e, nil
}

// loadEmail reads the notification settings.
func loadEmail() (EmailConfig, error) {
	var (
		e   EmailConfig
		err error
	)

	e.Enabled, err = envBool("EMAIL_ENABLED", true)
	if err != nil {
		return EmailConfig{}, err
	}
	// Read with os.Getenv rather than through env(): the key must not pick up
	// any fallback value, and it is never echoed anywhere.
	e.BrevoAPIKey = strings.TrimSpace(os.Getenv("BREVO_API_KEY"))
	e.BrevoSenderEmail = env("BREVO_SENDER_EMAIL", "")
	e.BrevoSenderName = env("BREVO_SENDER_NAME", "BeautyScale")

	if e.PollInterval, err = envDuration("EMAIL_POLL_INTERVAL", 10*time.Second); err != nil {
		return EmailConfig{}, err
	}
	if e.BatchSize, err = envInt32("EMAIL_BATCH_SIZE", 10); err != nil {
		return EmailConfig{}, err
	}
	concurrency, err := envInt32("EMAIL_CONCURRENCY", 2)
	if err != nil {
		return EmailConfig{}, err
	}
	e.Concurrency = int(concurrency)
	if e.SendTimeout, err = envDuration("EMAIL_SEND_TIMEOUT", 10*time.Second); err != nil {
		return EmailConfig{}, err
	}
	if e.MaxAttempts, err = envInt32("EMAIL_MAX_ATTEMPTS", 5); err != nil {
		return EmailConfig{}, err
	}
	if e.RetryBaseDelay, err = envDuration("EMAIL_RETRY_BASE_DELAY", 30*time.Second); err != nil {
		return EmailConfig{}, err
	}
	if e.RetryMaxDelay, err = envDuration("EMAIL_RETRY_MAX_DELAY", 30*time.Minute); err != nil {
		return EmailConfig{}, err
	}
	if e.StaleClaimAfter, err = envDuration("EMAIL_STALE_CLAIM_AFTER", 5*time.Minute); err != nil {
		return EmailConfig{}, err
	}
	return e, nil
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
	// A key without a sender address cannot send anything, and would spend
	// every event's attempt budget discovering that.
	if strings.TrimSpace(c.Email.BrevoAPIKey) != "" && strings.TrimSpace(c.Email.BrevoSenderEmail) == "" {
		return errors.New("config: BREVO_SENDER_EMAIL is required when BREVO_API_KEY is set")
	}
	if c.Email.BatchSize < 1 {
		return errors.New("config: EMAIL_BATCH_SIZE must be at least 1")
	}
	if c.Email.Concurrency < 1 {
		return errors.New("config: EMAIL_CONCURRENCY must be at least 1")
	}
	if c.Email.MaxAttempts < 1 {
		return errors.New("config: EMAIL_MAX_ATTEMPTS must be at least 1")
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

func envBool(key string, fallback bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("config: %s must be true or false: %w", key, err)
	}
	return v, nil
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
