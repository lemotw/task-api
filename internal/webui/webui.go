// Package webui serves the test console embedded in the binary.
// See docs/spec/extensions/11-web-ui.md.
package webui

import (
	_ "embed"
	"net/http"
)

//go:embed index.html
var indexHTML []byte

const methodNotAllowedBody = `{"error":"method not allowed"}` + "\n"

// Handler serves GET / (UI-1) and answers other methods with 405 (UI-2).
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(methodNotAllowedBody))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
}
