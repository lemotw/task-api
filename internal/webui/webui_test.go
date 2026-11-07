package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func serve(method string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(method, "/", nil))
	return rec
}

func TestGetRoot_ServesHTML(t *testing.T) {
	rec := serve(http.MethodGet)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "<html") {
		t.Errorf("body is not an HTML document: %q", rec.Body.String())
	}
}

func TestGetRoot_LoadsNoExternalResources(t *testing.T) {
	external := regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}`)
	if found := external.FindString(serve(http.MethodGet).Body.String()); found != "" {
		t.Errorf("found external reference %q", found)
	}
}

func TestOtherMethods_ReturnMethodNotAllowed(t *testing.T) {
	methods := []string{
		http.MethodHead, http.MethodOptions, http.MethodPost,
		http.MethodPut, http.MethodDelete, http.MethodPatch,
	}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			rec := serve(method)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d", rec.Code)
			}
			if got := rec.Header().Get("Allow"); got != "GET" {
				t.Errorf("Allow = %q", got)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q", got)
			}
			if method == http.MethodHead {
				return // a real server sends no body for HEAD
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error == "" {
				t.Errorf("body = %q, err = %v", rec.Body.String(), err)
			}
		})
	}
}
