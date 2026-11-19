package startup

import "testing"

func TestURL(t *testing.T) {
	cases := map[string]string{
		":8081":          "http://localhost:8081",
		"0.0.0.0:8081":   "http://localhost:8081",
		"[::]:8081":      "http://localhost:8081",
		"127.0.0.1:8081": "http://127.0.0.1:8081",
		"localhost:3000": "http://localhost:3000",
		"api.local:8080": "http://api.local:8080",
	}
	for addr, want := range cases {
		if got := URL(addr); got != want {
			t.Errorf("URL(%q) = %q, want %q", addr, got, want)
		}
	}
}
