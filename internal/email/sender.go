package email

import (
	"context"
	"fmt"
	"log/slog"
)

// Sender delivers one message. Implementations must respect the context's
// deadline: the worker bounds every send.
type Sender interface {
	// Send returns the provider's message ID when it has one. An error
	// wrapped in PermanentError must not be retried.
	Send(ctx context.Context, msg Message) (messageID string, err error)
	// Name identifies the implementation in logs.
	Name() string
}

// PermanentError marks a failure that retrying cannot fix: a malformed
// address, a rejected sender, a credential the provider refuses. The worker
// fails such an event immediately instead of spending its attempt budget.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// permanent wraps err so the worker stops retrying it.
func permanent(format string, args ...any) error {
	return &PermanentError{Err: fmt.Errorf(format, args...)}
}

// LogSender writes the message to the log instead of sending it.
//
// The assessment accepts console logging, so this is the default when no
// Brevo credentials are configured: the whole outbox and worker path still
// runs end to end, and swapping in the real provider is one environment
// variable. The body is logged, the recipient is logged, and no credential
// exists here to leak.
type LogSender struct {
	logger *slog.Logger
}

// NewLogSender returns a Sender that logs messages.
func NewLogSender(logger *slog.Logger) *LogSender {
	return &LogSender{logger: logger}
}

func (s *LogSender) Name() string { return "log" }

func (s *LogSender) Send(ctx context.Context, msg Message) (string, error) {
	s.logger.InfoContext(ctx, "email (not sent: no provider configured)",
		"to", msg.ToEmail,
		"toName", msg.ToName,
		"subject", msg.Subject,
		"body", msg.Text,
	)
	return "", nil
}
