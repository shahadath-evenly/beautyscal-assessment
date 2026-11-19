package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DefaultMaxBodyBytes bounds a JSON request body. The API accepts order
// requests with many lines but nothing large, so a small limit keeps a
// runaway or hostile client from consuming memory.
const DefaultMaxBodyBytes int64 = 1 << 20 // 1 MiB

// DecodeError describes why a request body could not be decoded, carrying the
// status and code the handler should return.
type DecodeError struct {
	Status  int
	Code    string
	Message string
}

func (e *DecodeError) Error() string { return e.Message }

// WriteTo sends the decode failure as an error envelope.
func (e *DecodeError) WriteTo(w http.ResponseWriter) {
	Error(w, e.Status, e.Code, e.Message, nil)
}

// DecodeJSON reads exactly one JSON object from the request body into dst.
//
// It rejects unknown fields, so a typo in a client payload is reported rather
// than silently ignored, and it rejects trailing content, so a body cannot
// smuggle a second document past validation. The body is bounded by
// MaxBytesReader; the router applies the same limit, and repeating it here
// keeps the function safe when called directly.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		if media := strings.TrimSpace(strings.Split(ct, ";")[0]); media != "application/json" {
			return &DecodeError{
				Status:  http.StatusUnsupportedMediaType,
				Code:    CodeBadRequest,
				Message: "Content-Type must be application/json.",
			}
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, DefaultMaxBodyBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return decodeFailure(err)
	}

	// Anything after the first document is a malformed request.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return &DecodeError{
			Status:  http.StatusBadRequest,
			Code:    CodeBadRequest,
			Message: "The request body must contain a single JSON object.",
		}
	}
	return nil
}

func decodeFailure(err error) *DecodeError {
	var (
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
		maxErr    *http.MaxBytesError
	)

	switch {
	case errors.As(err, &maxErr):
		return &DecodeError{
			Status:  http.StatusRequestEntityTooLarge,
			Code:    CodeRequestTooLarge,
			Message: fmt.Sprintf("The request body must be %d bytes or smaller.", maxErr.Limit),
		}

	case errors.As(err, &syntaxErr):
		return &DecodeError{
			Status:  http.StatusBadRequest,
			Code:    CodeBadRequest,
			Message: fmt.Sprintf("The request body contains malformed JSON at position %d.", syntaxErr.Offset),
		}

	case errors.As(err, &typeErr):
		return &DecodeError{
			Status: http.StatusBadRequest,
			Code:   CodeBadRequest,
			Message: fmt.Sprintf("Field %q must be of type %s.",
				typeErr.Field, typeErr.Type.String()),
		}

	case errors.Is(err, io.EOF):
		return &DecodeError{
			Status:  http.StatusBadRequest,
			Code:    CodeBadRequest,
			Message: "The request body must not be empty.",
		}

	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.TrimPrefix(err.Error(), "json: unknown field ")
		return &DecodeError{
			Status:  http.StatusBadRequest,
			Code:    CodeBadRequest,
			Message: fmt.Sprintf("The request body contains an unknown field %s.", field),
		}

	default:
		return &DecodeError{
			Status:  http.StatusBadRequest,
			Code:    CodeBadRequest,
			Message: "The request body could not be decoded.",
		}
	}
}
