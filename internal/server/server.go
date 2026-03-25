// Package server wires up the HTTP router and all middleware.
package server

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mkende/golink-redirector/internal/auth"
	"github.com/mkende/golink-redirector/internal/config"
	"github.com/mkende/golink-redirector/internal/db"
	serverMiddleware "github.com/mkende/golink-redirector/internal/server/middleware"
)

// Server is the root HTTP handler.
type Server struct {
	router  chi.Router
	cfg     *config.Config
	links   db.LinkRepo
	users   db.UserRepo
	logger  *slog.Logger
	oidcH   *auth.OIDCHandler
}

// New creates a new Server and wires up all routes. The oidcHandler may be nil
// when OIDC is disabled.
func New(cfg *config.Config, sqlDB *sql.DB, logger *slog.Logger, oidcHandler *auth.OIDCHandler) http.Handler {
	s := &Server{
		cfg:    cfg,
		links:  db.NewLinkRepo(sqlDB),
		users:  db.NewUserRepo(sqlDB),
		logger: logger,
		oidcH:  oidcHandler,
	}
	s.router = s.buildRouter()
	return s.router
}

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	// Standard middleware
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(serverMiddleware.RequestLogger(s.logger))

	// Auth middleware: populate identity context from Tailscale headers or JWT cookie
	r.Use(auth.TailscaleMiddleware(s.cfg, s.users))
	r.Use(auth.OIDCMiddleware(s.cfg))

	// Health check (no domain redirect)
	r.Get("/healthz", s.handleHealthz)

	// Auth routes (no domain redirect; OIDC callback must be reachable on the
	// registered redirect URL regardless of the current hostname)
	if s.oidcH != nil {
		r.Get("/auth/login", s.oidcH.HandleLogin)
		r.Get("/auth/callback", s.oidcH.HandleCallback)
		r.Post("/auth/logout", s.oidcH.HandleLogout)
		r.Get("/auth/logout", s.oidcH.HandleLogout)
	}

	// Redirect routes bypass domain redirect middleware
	r.Get("/{name}", s.handleRedirect)
	r.Get("/{name}/*", s.handleRedirect)

	// All other routes: apply domain redirect middleware
	r.Group(func(r chi.Router) {
		r.Use(serverMiddleware.DomainRedirect(s.cfg))
		// Future: UI routes, API routes go here
	})

	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok")) //nolint:errcheck
}
