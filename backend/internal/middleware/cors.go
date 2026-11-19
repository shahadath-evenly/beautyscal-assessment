// Package middleware holds the HTTP middleware shared by the API, including
// the workspace scoping middleware added in a later phase.
package middleware

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CORS allows the configured browser origins to call the API with the headers
// this application relies on, notably X-Workspace-ID. An empty allowlist
// disables cross-origin responses entirely. Implemented here rather than
// pulling in another dependency, since the policy is this small.
func CORS(allowedOrigins []string, maxAge time.Duration) func(http.Handler) http.Handler {
	allowAll := slices.Contains(allowedOrigins, "*")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := origin != "" && (allowAll || slices.Contains(allowedOrigins, origin))

			if allowed {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", strings.Join([]string{
					"Content-Type", "X-Workspace-ID",
				}, ", "))
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(maxAge.Seconds())))
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
