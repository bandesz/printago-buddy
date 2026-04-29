package web

import (
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/bandesz/printago-buddy/internal/printago"
)

//go:embed templates
var templateFS embed.FS

// Server serves the Printago Buddy web UI.
type Server struct {
	client    printago.ClientInterface
	port      int
	queueTmpl *template.Template
}

// NewServer parses templates and creates a Server. Returns an error if any
// template fails to parse.
func NewServer(client printago.ClientInterface, port int) (*Server, error) {
	base, err := template.ParseFS(templateFS, "templates/base.html")
	if err != nil {
		return nil, fmt.Errorf("parse base template: %w", err)
	}

	queueClone, err := base.Clone()
	if err != nil {
		return nil, fmt.Errorf("clone base template: %w", err)
	}
	queueTmpl, err := queueClone.ParseFS(templateFS, "templates/queue.html")
	if err != nil {
		return nil, fmt.Errorf("parse queue template: %w", err)
	}

	return &Server{client: client, port: port, queueTmpl: queueTmpl}, nil
}

// Start registers routes and blocks serving HTTP on the configured port.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/queue", s.handleQueue)

	addr := fmt.Sprintf(":%d", s.port)
	slog.Info("web server started", "addr", "http://localhost"+addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) render(w http.ResponseWriter, tmpl *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		slog.Error("web: template render error", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
