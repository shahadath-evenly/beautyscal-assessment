package email_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/real-musafir/b-scale/backend/internal/email"
)

const testKey = "xkeysib-test-key-never-logged"

func newTestSender(t *testing.T, handler http.HandlerFunc) *email.BrevoSender {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	sender, err := email.NewBrevoSender(email.BrevoConfig{
		APIKey:      testKey,
		SenderEmail: "sales@beautyscale.test",
		SenderName:  "BeautyScale Sales",
		Endpoint:    server.URL,
		Timeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBrevoSender: %v", err)
	}
	return sender
}

func TestBrevoSendPostsTheExpectedRequest(t *testing.T) {
	var (
		gotKey    string
		gotMethod string
		gotBody   map[string]any
	)

	sender := newTestSender(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("api-key")
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"messageId":"<202611.1@brevo>"}`))
	})

	id, err := sender.Send(context.Background(), email.Message{
		ToEmail: "ada@example.test",
		ToName:  "Ada Lovelace",
		Subject: "Your order request has been accepted",
		Text:    "plain",
		HTML:    "<p>rich</p>",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "<202611.1@brevo>" {
		t.Errorf("messageId = %q", id)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s", gotMethod)
	}
	// The key travels in the header Brevo expects, and only there.
	if gotKey != testKey {
		t.Errorf("api-key header = %q", gotKey)
	}
	if gotBody["subject"] != "Your order request has been accepted" {
		t.Errorf("subject = %v", gotBody["subject"])
	}
	if gotBody["textContent"] != "plain" || gotBody["htmlContent"] != "<p>rich</p>" {
		t.Errorf("bodies = %v / %v", gotBody["textContent"], gotBody["htmlContent"])
	}
	sender2, _ := gotBody["sender"].(map[string]any)
	if sender2["email"] != "sales@beautyscale.test" {
		t.Errorf("sender = %v", sender2)
	}
	to, _ := gotBody["to"].([]any)
	if len(to) != 1 {
		t.Fatalf("to = %v", to)
	}
	if first, _ := to[0].(map[string]any); first["email"] != "ada@example.test" {
		t.Errorf("recipient = %v", to[0])
	}
}

// A 2xx without a parsable message ID is still a delivery: the provider
// accepted it, and inventing a failure would send the message twice.
func TestBrevoSendAcceptsAResponseWithoutAMessageID(t *testing.T) {
	sender := newTestSender(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`not json`))
	})

	id, err := sender.Send(context.Background(), email.Message{ToEmail: "a@b.test"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "" {
		t.Errorf("messageId = %q, want empty", id)
	}
}

func TestBrevoSendClassifiesFailures(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		permanent bool
	}{
		{"bad request is permanent", http.StatusBadRequest, `{"message":"Invalid email"}`, true},
		{"unauthorised is permanent", http.StatusUnauthorized, `{"message":"Key not found"}`, true},
		{"rate limited is retryable", http.StatusTooManyRequests, `{"message":"slow down"}`, false},
		{"server error is retryable", http.StatusInternalServerError, `oops`, false},
		{"gateway error is retryable", http.StatusBadGateway, ``, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sender := newTestSender(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			_, err := sender.Send(context.Background(), email.Message{ToEmail: "a@b.test"})
			if err == nil {
				t.Fatal("expected an error")
			}

			var permanent *email.PermanentError
			if got := errors.As(err, &permanent); got != tc.permanent {
				t.Fatalf("permanent = %v, want %v (error: %v)", got, tc.permanent, err)
			}
			// Whatever happened, the key must not be in the message a
			// human or a log will see.
			if strings.Contains(err.Error(), testKey) {
				t.Fatal("the error message leaks the API key")
			}
		})
	}
}

// A provider that never answers must not hold a worker slot forever.
func TestBrevoSendRespectsTheContextDeadline(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	sender := newTestSender(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := sender.Send(ctx, email.Message{ToEmail: "a@b.test"})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Send took %v, deadline was 150ms", elapsed)
	}
	// A timeout is transient: the next attempt may well succeed.
	var permanent *email.PermanentError
	if errors.As(err, &permanent) {
		t.Error("a timeout must not be classified as permanent")
	}
}

func TestNewBrevoSenderRequiresCredentials(t *testing.T) {
	if _, err := email.NewBrevoSender(email.BrevoConfig{SenderEmail: "a@b.test"}); err == nil {
		t.Error("expected an error without an API key")
	}
	if _, err := email.NewBrevoSender(email.BrevoConfig{APIKey: testKey}); err == nil {
		t.Error("expected an error without a sender address")
	}
}
