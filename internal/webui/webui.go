// Package webui serves the test console embedded in the binary.
// See docs/spec/extensions/11-web-ui.md.
package webui

import "net/http"

// Handler serves GET / (UI-1) and answers other methods with 405 (UI-2).
func Handler() http.Handler {
	panic("not implemented")
}
