package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// DefaultEnvFile is the file Load reads when ENV_FILE is not set.
const DefaultEnvFile = ".env"

// loadEnvFile reads a KEY=VALUE file and sets any variable that is not
// already present in the process environment, so a real environment variable
// (or a Docker Compose value) always wins over the file.
//
// It returns the number of variables actually injected, which startup output
// reports, and whether the file existed.
//
// A missing file is not an error: containers get their configuration from the
// environment and have no .env at all. Parsed with a small reader rather than
// a dependency, since the format needed here is this simple.
func loadEnvFile(path string) (injected int, found bool, err error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		key, value, ok := parseEnvLine(scanner.Text())
		if !ok {
			continue
		}
		if key == "" {
			return 0, true, fmt.Errorf("config: %s:%d: missing variable name", path, lineNo)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return 0, true, fmt.Errorf("config: set %s: %w", key, err)
		}
		injected++
	}
	if err := scanner.Err(); err != nil {
		return 0, true, fmt.Errorf("config: read %s: %w", path, err)
	}
	return injected, true, nil
}

// parseEnvLine splits one line into a key and value. It reports false for
// blank lines and comments, which are skipped.
func parseEnvLine(line string) (key, value string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}

	// Tolerate lines written for a shell, e.g. `export FOO=bar`.
	trimmed = strings.TrimPrefix(trimmed, "export ")

	name, raw, found := strings.Cut(trimmed, "=")
	if !found {
		return "", "", false
	}

	key = strings.TrimSpace(name)
	value = strings.TrimSpace(raw)

	// Strip one layer of matching quotes. An unquoted value keeps any inline
	// comment intact, because a bare # is legal inside a connection string.
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}
