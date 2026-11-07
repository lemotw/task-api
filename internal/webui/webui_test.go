package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// titles identifies each page by its document title (UI-1).
var titles = map[string]string{
	"/":     "<title>Tasks</title>",
	"/test": "<title>Task API Console</title>",
}

func serve(t *testing.T, path, method string) *httptest.ResponseRecorder {
	t.Helper()
	page, ok := Pages()[path]
	if !ok {
		t.Fatalf("no page for %s", path)
	}
	rec := httptest.NewRecorder()
	page.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestPages_OwnExactlyTheUIPaths(t *testing.T) {
	var paths []string
	for path := range Pages() {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if strings.Join(paths, ",") != "/,/test" {
		t.Errorf("paths = %v, want / and /test", paths)
	}
}

func TestPages_ServeHTML(t *testing.T) {
	for path, title := range titles {
		t.Run(path, func(t *testing.T) {
			rec := serve(t, path, http.MethodGet)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
				t.Errorf("Content-Type = %q", got)
			}
			if !strings.Contains(rec.Body.String(), title) {
				t.Errorf("title %q not found", title)
			}
		})
	}
}

func TestPages_LoadNoExternalResources(t *testing.T) {
	external := regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}`)
	for path := range titles {
		if found := external.FindString(serve(t, path, http.MethodGet).Body.String()); found != "" {
			t.Errorf("%s: found external reference %q", path, found)
		}
	}
}

// Names are shown as stored, including repeated spaces and newlines (UI-25).
func TestPages_PreserveWhitespaceInNames(t *testing.T) {
	for path := range titles {
		if body := serve(t, path, http.MethodGet).Body.String(); !strings.Contains(body, "white-space: pre-wrap") {
			t.Errorf("%s: names are not rendered with white-space: pre-wrap", path)
		}
	}
}

func TestPages_LinkToEachOther(t *testing.T) {
	links := map[string]string{"/": `href="/test"`, "/test": `href="/"`}
	for path, link := range links {
		if body := serve(t, path, http.MethodGet).Body.String(); !strings.Contains(body, link) {
			t.Errorf("%s: no link %s", path, link)
		}
	}
}

func TestPages_OtherMethodsReturnMethodNotAllowed(t *testing.T) {
	methods := []string{
		http.MethodHead, http.MethodOptions, http.MethodPost,
		http.MethodPut, http.MethodDelete, http.MethodPatch,
	}
	for path := range titles {
		for _, method := range methods {
			t.Run(path+" "+method, func(t *testing.T) {
				rec := serve(t, path, method)
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
}
