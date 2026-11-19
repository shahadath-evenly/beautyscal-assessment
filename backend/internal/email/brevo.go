package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// brevoEndpoint is Brevo's transactional email API.
const brevoEndpoint = "https://api.brevo.com/v3/smtp/email"

// BrevoSender delivers messages through Brevo's transactional API.
//
// The API key lives only in this struct and in the one header it sets. It is
// never logged, never placed in a URL and never returned in an error: the
// error paths below quote the status code and the response body, and Brevo
// does not echo the key back.
type BrevoSender struct {
	apiKey      string
	senderEmail string
	senderName  string
	endpoint    string
	client      *http.Client
}

// BrevoConfig is what the sender needs. Timeout bounds a single API call; the
// worker also passes a context deadline, and whichever is shorter wins.
type BrevoConfig struct {
	APIKey      string
	SenderEmail string
	SenderName  string
	Timeout     time.Duration
	// Endpoint overrides the API URL. Only tests set it.
	Endpoint string
}

// NewBrevoSender validates the configuration and returns a sender. It fails
// rather than starting with half a configuration, because a worker that
// cannot possibly succeed should not consume the attempt budget of every
// queued event.
func NewBrevoSender(cfg BrevoConfig) (*BrevoSender, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("email: BREVO_API_KEY is required for the brevo sender")
	}
	if strings.TrimSpace(cfg.SenderEmail) == "" {
		return nil, fmt.Errorf("email: BREVO_SENDER_EMAIL is required for the brevo sender")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = brevoEndpoint
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	name := strings.TrimSpace(cfg.SenderName)
	if name == "" {
		name = "BeautyScale"
	}

	return &BrevoSender{
		apiKey:      cfg.APIKey,
		senderEmail: strings.TrimSpace(cfg.SenderEmail),
		senderName:  name,
		endpoint:    endpoint,
		client:      &http.Client{Timeout: timeout},
	}, nil
}

func (s *BrevoSender) Name() string { return "brevo" }

type brevoContact struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type brevoPayload struct {
	Sender      brevoContact   `json:"sender"`
	To          []brevoContact `json:"to"`
	Subject     string         `json:"subject"`
	HTMLContent string         `json:"htmlContent"`
	TextContent string         `json:"textContent"`
}

type brevoResponse struct {
	MessageID string `json:"messageId"`
}

// Send posts one message to Brevo.
//
// Failures are classified, because the distinction decides whether the event
// is retried: 4xx other than 429 means the request itself is wrong and will
// be wrong again, so it is permanent; 429 and 5xx are transient, as is any
// transport error.
func (s *BrevoSender) Send(ctx context.Context, msg Message) (string, error) {
	payload := brevoPayload{
		Sender:      brevoContact{Email: s.senderEmail, Name: s.senderName},
		To:          []brevoContact{{Email: msg.ToEmail, Name: msg.ToName}},
		Subject:     msg.Subject,
		HTMLContent: msg.HTML,
		TextContent: msg.Text,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		// A message that cannot be encoded will never encode.
		return "", permanent("email: encode brevo payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", permanent("email: build brevo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// The only place the key appears.
	req.Header.Set("api-key", s.apiKey)

	res, err := s.client.Do(req)
	if err != nil {
		// Transport failure: unreachable, reset, timed out. Worth retrying.
		return "", fmt.Errorf("email: call brevo: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	// Bounded read: a provider must not be able to exhaust memory here.
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))

	switch {
	case res.StatusCode >= 200 && res.StatusCode < 300:
		var parsed brevoResponse
		// A missing or unparsable message ID is not a delivery failure: the
		// provider accepted the message, which is what the status says.
		_ = json.Unmarshal(raw, &parsed)
		return parsed.MessageID, nil

	case res.StatusCode == http.StatusTooManyRequests,
		res.StatusCode >= 500:
		return "", fmt.Errorf("email: brevo returned %d: %s", res.StatusCode, summarise(raw))

	default:
		// 400, 401, 403: the payload, the key or the sender is wrong.
		// Retrying sends the same thing again.
		return "", permanent("email: brevo rejected the message with %d: %s",
			res.StatusCode, summarise(raw))
	}
}

// summarise trims a provider response to something safe and short for a log
// line or a stored error.
func summarise(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	s = strings.Join(strings.Fields(s), " ")
	const max = 300
	if len(s) > max {
		return s[:max] + "…"
	}
	if s == "" {
		return "(empty response)"
	}
	return s
}
