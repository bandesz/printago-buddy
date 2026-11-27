package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/bandesz/printago-buddy/internal/printago"
	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

//go:embed templates
var templateFS embed.FS

// queueScheduler is the smart queue surface the queue page reads and the switch and Prioritize post to.
// *smartqueue.Scheduler implements it.
type queueScheduler interface {
	Snapshot() *smartqueue.Plan
	Enable(ctx context.Context) error
	Disable(ctx context.Context) error
	Prioritize(ctx context.Context, jobID string) error
	HalfLife() string
	SetHalfLife(ctx context.Context, raw string) error
}

// Server serves the Printago Buddy web UI.
type Server struct {
	client    printago.ClientInterface
	scheduler queueScheduler
	port      int
	queueTmpl *template.Template
}

// NewServer parses templates and creates a Server. Returns an error if any
// template fails to parse. scheduler is stored for the queue page.
func NewServer(client printago.ClientInterface, scheduler *smartqueue.Scheduler, port int) (*Server, error) {
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

	var stored queueScheduler
	if scheduler != nil {
		stored = scheduler
	}
	return &Server{client: client, scheduler: stored, port: port, queueTmpl: queueTmpl}, nil
}

// Start registers routes and blocks serving HTTP on the configured port.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	slog.Info("web server started", "addr", "http://localhost"+addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/queue", s.handleQueue)
	mux.HandleFunc("/smart-queue", s.handleSmartQueue)
	mux.HandleFunc("/cancel-job", s.handleCancelJob)
	mux.HandleFunc("/prioritize-job", s.handlePrioritizeJob)
	mux.HandleFunc("/half-life", s.handleHalfLife)
	return mux
}

func (s *Server) render(w http.ResponseWriter, tmpl *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		slog.Error("web: template render error", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
