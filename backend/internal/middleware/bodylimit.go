package middleware

import (
	"net/http"
)

// MaxBodyBytes caps the size of every request body before a handler reads it.
// Reading past the limit fails the read rather than buffering, so a large or
// endless body cannot exhaust memory.
func MaxBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
