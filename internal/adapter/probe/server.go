package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	statusField              = "status"
	defaultReadHeaderTimeout = 5 * time.Second
	readinessTimeout         = 3 * time.Second
)

// CheckFunc defines a readiness verification check (e.g. Postgres ping, broker ping).
type CheckFunc func(ctx context.Context) error

// Server is a lightweight standalone HTTP server serving /livez and /readyz probes.
type Server struct {
	httpServer *http.Server
	checks     map[string]CheckFunc
	logger     *slog.Logger
}

// Config configures the probe HTTP server.
type Config struct {
	Port   int
	Logger *slog.Logger
}

// NewServer initializes a new probe Server.
func NewServer(cfg Config) *Server {
	if cfg.Port <= 0 {
		cfg.Port = 8081
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	s := &Server{
		checks: make(map[string]CheckFunc),
		logger: cfg.Logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", s.handleLiveness)
	mux.HandleFunc("GET /readyz", s.handleReadiness)

	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	return s
}

// RegisterCheck registers a named dependency readiness check.
func (s *Server) RegisterCheck(name string, check CheckFunc) {
	s.checks[name] = check
}

// Start begins listening on the configured probe port.
func (s *Server) Start() error {
	s.logger.Info("starting probe health server", "addr", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("probe server listen error: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the probe server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down probe server")
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{statusField: "alive"})
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	failures := make(map[string]string)
	for name, check := range s.checks {
		if err := check(ctx); err != nil {
			failures[name] = err.Error()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if len(failures) > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			statusField: "not ready",
			"failures":  failures,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{statusField: "ready"})
}

// Addr returns the configured listening address.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

// Serve starts serving on a custom listener (helpful for testing with dynamic port).
func (s *Server) Serve(l net.Listener) error {
	if err := s.httpServer.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
