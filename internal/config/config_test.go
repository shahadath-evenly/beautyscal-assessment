package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	content := "" +
		"# a comment\n" +
		"\n" +
		"DATABASE_URL=postgres://user@localhost:5432/db?sslmode=disable\n" +
		"export LOG_LEVEL=debug\n" +
		"QUOTED=\"double\"\n" +
		"SINGLE='single'\n" +
		"ALREADY_SET=from_file\n" +
		"  SPACED = padded \n" +
		"NOT_AN_ASSIGNMENT\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp env file: %v", err)
	}

	// A real environment variable must win over the file.
	t.Setenv("ALREADY_SET", "from_environment")

	for _, key := range []string{"DATABASE_URL", "LOG_LEVEL", "QUOTED", "SINGLE", "SPACED"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}

	injected, found, err := loadEnvFile(path)
	if err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if !found {
		t.Error("found = false, want true for an existing file")
	}
	// Six assignments parse; ALREADY_SET is skipped because the environment
	// already holds it, so five are injected.
	if injected != 5 {
		t.Errorf("injected = %d, want 5", injected)
	}

	want := map[string]string{
		"DATABASE_URL": "postgres://user@localhost:5432/db?sslmode=disable",
		"LOG_LEVEL":    "debug",
		"QUOTED":       "double",
		"SINGLE":       "single",
		"SPACED":       "padded",
		"ALREADY_SET":  "from_environment",
	}
	for key, expected := range want {
		if got := os.Getenv(key); got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}
}

func TestLoadEnvFileMissingIsNotAnError(t *testing.T) {
	injected, found, err := loadEnvFile(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("a missing file must be ignored, got: %v", err)
	}
	if found || injected != 0 {
		t.Errorf("found = %v, injected = %d; want false, 0", found, injected)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	// Point ENV_FILE at a path that does not exist, to isolate Load from any
	// real .env in the working directory.
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error when DATABASE_URL is empty")
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("DATABASE_URL", "postgres://user@localhost:5432/db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPPort != "8081" {
		t.Errorf("HTTPPort = %q, want 8081", cfg.HTTPPort)
	}
	if cfg.Addr() != ":8081" {
		t.Errorf("Addr() = %q, want :8081", cfg.Addr())
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("DBMaxConns = %d, want 10", cfg.DBMaxConns)
	}
}

func TestLoadExpiryDefaultsToSevenDays(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("DATABASE_URL", "postgres://user@localhost:5432/db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Expiry.Enabled {
		t.Error("Expiry.Enabled = false, want true")
	}
	if cfg.Expiry.MaxAge != 7*24*time.Hour {
		t.Errorf("Expiry.MaxAge = %v, want 168h", cfg.Expiry.MaxAge)
	}
	if cfg.Expiry.Interval != time.Hour {
		t.Errorf("Expiry.Interval = %v, want 1h", cfg.Expiry.Interval)
	}
}

// A window of zero would expire every request on the next sweep, including
// one submitted a second ago, so it is rejected at startup rather than
// defaulted away.
func TestLoadRejectsANonPositiveExpiryWindow(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("DATABASE_URL", "postgres://user@localhost:5432/db")
	t.Setenv("EXPIRY_MAX_AGE", "0s")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error when EXPIRY_MAX_AGE is zero")
	}
}

// The job can be turned off entirely.
func TestLoadCanDisableExpiry(t *testing.T) {
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "absent"))
	t.Setenv("DATABASE_URL", "postgres://user@localhost:5432/db")
	t.Setenv("EXPIRY_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Expiry.Enabled {
		t.Error("Expiry.Enabled = true, want false")
	}
}
