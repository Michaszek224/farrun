// Package web serves the farrun user interface over HTTP.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/michaszek224/farrun/internal/store"
)

//go:embed templates static
var assets embed.FS

// pageNames are the templates in templates/, each combined with layout.html.
var pageNames = []string{"list", "detail", "error"}

type server struct {
	store *store.Store
	loc   *time.Location
	pages map[string]*template.Template
}

// NewHandler returns the application's HTTP handler. Times are shown in loc.
func NewHandler(st *store.Store, loc *time.Location) (http.Handler, error) {
	s := &server{store: st, loc: loc, pages: map[string]*template.Template{}}

	// Every page gets its own template set, so the "title" and "content"
	// blocks of different pages cannot collide.
	for _, name := range pageNames {
		t, err := template.New(name).Funcs(s.funcs()).ParseFS(assets, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
		s.pages[name] = t
	}

	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleRoot)
	mux.HandleFunc("GET /activities", s.handleList)
	mux.HandleFunc("GET /activities/{id}", s.handleDetail)
	mux.HandleFunc("GET /api/activities/{id}/series", s.handleSeries)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	return secure(mux), nil
}

// secure adds headers that keep the pages self-contained: no inline or
// third-party scripts or styles, and no referrer leaking to other sites.
func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// render executes the page into a buffer first, so a template error produces a
// clean 500 instead of a half-written page.
func (s *server) render(w http.ResponseWriter, r *http.Request, page string, status int, data any) {
	var buf bytes.Buffer
	if err := s.pages[page].ExecuteTemplate(&buf, "layout", data); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (s *server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	slogError(r, err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func slogError(r *http.Request, err error) {
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
}

// errorPage shows a friendly message in the normal layout.
func (s *server) errorPage(w http.ResponseWriter, r *http.Request, status int, message string) {
	s.render(w, r, "error", status, map[string]any{
		"Status":  status,
		"Title":   http.StatusText(status),
		"Message": message,
	})
}

func (s *server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/activities", http.StatusFound)
}
