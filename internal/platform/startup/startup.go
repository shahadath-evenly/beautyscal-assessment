// Package startup reports the few milestones a developer wants to see when a
// command boots: which env file was read, that the database answered, and the
// address the server is actually listening on.
//
// In human mode it prints short readable lines to stdout. Otherwise the same
// milestones go through the structured logger, which is what containers and
// log aggregators consume. Ongoing application logging always uses the
// logger; this package only covers startup.
package startup

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
)

// Reporter prints or logs startup milestones.
type Reporter struct {
	logger *slog.Logger
	out    io.Writer
	human  bool
}

// New returns a Reporter. When human is true the milestones are printed for a
// person reading a terminal; when false they are logged as structured events.
func New(logger *slog.Logger, human bool) *Reporter {
	return &Reporter{logger: logger, out: os.Stdout, human: human}
}

// EnvLoaded reports the variables an env file contributed. It stays silent
// when no file was found, which is the normal case inside a container.
func (r *Reporter) EnvLoaded(file string, count int) {
	if file == "" {
		return
	}
	if r.human {
		fmt.Fprintf(r.out, "◇ injected env (%d) from %s\n", count, file)
		return
	}
	r.logger.Info("env file loaded", "file", file, "variables", count)
}

// DatabaseConnected reports a verified database connection.
func (r *Reporter) DatabaseConnected() {
	if r.human {
		fmt.Fprintln(r.out, "PostgreSQL connection established")
		return
	}
	r.logger.Info("postgresql connection established")
}

// Listening reports the address the server is bound to, as a URL that can be
// clicked in a terminal, with the environment name in parentheses.
func (r *Reporter) Listening(addr, appEnv string) {
	url := URL(addr)
	if r.human {
		fmt.Fprintf(r.out, "Backend server listening on %s (%s)\n", url, appEnv)
		return
	}
	r.logger.Info("backend server listening", "url", url, "env", appEnv)
}

// Done reports a clean shutdown.
func (r *Reporter) Done(message string) {
	if r.human {
		fmt.Fprintln(r.out, message)
		return
	}
	r.logger.Info(message)
}

// URL turns a listen address into a browsable URL. A wildcard or empty host
// becomes localhost, because that is the address a developer can actually
// open; 0.0.0.0 is not reachable in a browser.
func URL(addr string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		// Not a host:port pair, e.g. a bare ":8081" that failed to split.
		return "http://localhost" + ensureLeadingColon(addr)
	}
	switch host {
	case "", "0.0.0.0", "[::]", "::", "*":
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func ensureLeadingColon(addr string) string {
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	return ":" + addr
}
