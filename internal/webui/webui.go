// Package webui serves the pages embedded in the binary.
// See docs/spec/extensions/11-web-ui.md.
package webui

import (
	_ "embed"
	"net/http"
)

var (
	//go:embed app.html
	appHTML []byte

	//go:embed console.html
	consoleHTML []byte
)

const methodNotAllowedBody = `{"error":"method not allowed"}` + "\n"

// Pages returns the web UI pages keyed by path (UI-1, UI-7): the task page at
// / and the API test console at /test.
func Pages() map[string]http.Handler {
	return map[string]http.Handler{
		"/":     page(appHTML),
		"/test": page(consoleHTML),
	}
}

// page serves html for GET and answers other methods with 405 (UI-2).
func page(html []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(methodNotAllowedBody))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	})
}
