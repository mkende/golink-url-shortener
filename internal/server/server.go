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
	"github.com/mkende/golink-redirector/internal/templates"
)

// Server is the root HTTP handler.
type Server struct {
	router   chi.Router
	cfg      *config.Config
	links    db.LinkRepo
	users    db.UserRepo
	apiKeys  db.APIKeyRepo
	logger   *slog.Logger
	oidcH    *auth.OIDCHandler
	renderer *templates.Renderer
}

// New creates a new Server and wires up all routes. The oidcHandler may be nil
// when OIDC is disabled.
func New(cfg *config.Config, sqlDB *sql.DB, logger *slog.Logger, oidcHandler *auth.OIDCHandler) http.Handler {
	renderer, err := templates.New()
	if err != nil {
		// Template parse errors are programmer errors; panic early so they are
		// caught during development rather than silently serving broken pages.
		panic("failed to parse templates: " + err.Error())
	}

	s := &Server{
		cfg:      cfg,
		links:    db.NewLinkRepo(sqlDB),
		users:    db.NewUserRepo(sqlDB),
		apiKeys:  db.NewAPIKeyRepo(sqlDB),
		logger:   logger,
		oidcH:    oidcHandler,
		renderer: renderer,
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

		// Landing page
		r.Get("/", s.handleIndex)

		// Link creation
		r.Get("/new", s.handleNew)
		r.Post("/new", s.handleNew)

		// Link editing
		r.Get("/edit/{name}", s.handleEdit)
		r.Post("/edit/{name}", s.handleEdit)
		r.Post("/edit/{name}/share", s.handleEditShare)
		r.Post("/edit/{name}/unshare", s.handleEditUnshare)
		r.Post("/edit/{name}/delete", s.handleEditDelete)

		// Link browsing
		r.Get("/links", s.handleLinks)
		r.Get("/mylinks", s.handleMyLinks)

		// Help page
		r.Get("/help", s.handleHelp)

		// Admin: API key management UI
		r.Route("/apikeys", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg))
			r.Use(auth.RequireAdmin())
			r.Get("/", s.handleAPIKeysPage)
			r.Post("/", s.handleCreateAPIKey)
			r.Post("/{id}/delete", s.handleDeleteAPIKey)
		})
	})

	// API key middleware applied before the API sub-router so that API key
	// bearers get an Identity before RequireAuth runs.
	r.Group(func(r chi.Router) {
		r.Use(serverMiddleware.DomainRedirect(s.cfg))
		r.Use(s.APIKeyMiddleware)

		// REST API — all routes require authentication (session or API key).
		r.Route("/api", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg))

			// Link CRUD
			r.Get("/links", s.handleAPIListLinks)
			r.Post("/links", s.handleAPICreateLink)
			r.Get("/links/{name}", s.handleAPIGetLink)
			r.Patch("/links/{name}", s.handleAPIUpdateLink)
			r.Delete("/links/{name}", s.handleAPIDeleteLink)

			// Quick-name generator (also reachable by HTMX from the group above)
			r.Get("/quickname", s.handleQuickName)

			// API key management — admin only
			r.Route("/apikeys", func(r chi.Router) {
				r.Use(auth.RequireAdmin())
				r.Get("/", s.handleAPIListAPIKeys)
				r.Post("/", s.handleAPICreateAPIKey)
				r.Delete("/{id}", s.handleAPIDeleteAPIKey)
			})
		})
	})

	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok")) //nolint:errcheck
}
