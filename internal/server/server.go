// Package server wires up the HTTP router and all middleware.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mkende/golink-url-shortener/internal/config"
	"github.com/mkende/golink-url-shortener/internal/db"
	"github.com/mkende/golink-url-shortener/internal/templates"
	"github.com/mkende/golink-url-shortener/pkg/httpauth"
	"github.com/mkende/golink-url-shortener/web/static"
)

// Server is the root HTTP handler. It implements http.Handler via ServeHTTP.
type Server struct {
	router      chi.Router
	cfg         *config.Config
	links       db.LinkRepo
	users       db.UserRepo
	apiKeys     db.APIKeyRepo
	logger      *slog.Logger
	authManager *httpauth.AuthManager
	renderer    *templates.Renderer
	useCounter  *db.UseCounter
}

// New creates a new Server, wires up all routes, and starts background
// goroutines. Call Shutdown to drain them gracefully.
func New(cfg *config.Config, sqlDB *db.DB, logger *slog.Logger, authManager *httpauth.AuthManager) *Server {
	renderer, err := templates.New(logger)
	if err != nil {
		panic("failed to parse templates: " + err.Error())
	}

	cacheSize := cfg.CacheSize
	if cacheSize <= 0 {
		cacheSize = 1000
	}
	baseRepo := db.NewLinkRepo(sqlDB)
	cachingRepo, err := db.NewCachingLinkRepo(baseRepo, cacheSize, cfg.CacheTTLDuration)
	if err != nil {
		panic("failed to create link cache: " + err.Error())
	}

	s := &Server{
		cfg:         cfg,
		links:       cachingRepo,
		users:       db.NewUserRepo(sqlDB),
		apiKeys:     db.NewAPIKeyRepo(sqlDB),
		logger:      logger,
		authManager: authManager,
		renderer:    renderer,
		useCounter:  db.NewUseCounter(sqlDB, 2*time.Second),
	}
	s.router = s.buildRouter()
	return s
}

// ServeHTTP implements http.Handler, delegating to the underlying chi router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// Shutdown drains the use-count buffer and stops background goroutines. It
// should be called after the HTTP server has stopped accepting new requests.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.useCounter.Shutdown(ctx)
}

// logr returns the request-scoped logger if the log-enricher middleware has
// populated one in ctx, otherwise falls back to the server-level logger.
func (s *Server) logr(ctx context.Context) *slog.Logger {
	return httpauth.LoggerFromContext(ctx, s.logger)
}

// lookupAPIKey implements httpauth.APIKeyLookup. It looks up the key by hash
// and asynchronously updates last_used_at.
func (s *Server) lookupAPIKey(ctx context.Context, hash string) (*httpauth.APIKeyInfo, error) {
	key, err := s.apiKeys.GetByHash(ctx, hash)
	if err != nil {
		return nil, nil //nolint:nilerr // not found — pass through without identity
	}
	go func() {
		s.apiKeys.UpdateLastUsed(context.Background(), key.ID) //nolint:errcheck
	}()
	return &httpauth.APIKeyInfo{
		ID:       key.ID,
		Name:     key.Name,
		ReadOnly: key.ReadOnly,
	}, nil
}

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	// Global middleware: real-IP, logging, security headers, auth providers.
	r.Use(s.authManager.Middleware())

	// Static assets: served before domain redirect and auth so that login
	// pages and non-canonical-domain requests can still load assets.
	// Files are versioned in their names so responses are immutably cached.
	staticHandler := http.StripPrefix("/static", http.FileServer(http.FS(static.FS)))
	r.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		staticHandler.ServeHTTP(w, r)
	}))

	// Favicon — served before domain redirect so browsers always get it.
	r.Get("/favicon.ico", s.handleFavicon)

	// Health check (no domain redirect).
	r.Get("/healthz", s.handleHealthz)

	// Auth routes (no domain redirect; OIDC callback must be reachable on the
	// registered redirect URL regardless of the current hostname).
	s.authManager.Mount(r)

	// Redirect routes bypass domain redirect middleware.
	r.Get("/{name}", s.handleRedirect)
	r.Get("/{name}/*", s.handleRedirect)

	// UI routes: apply domain redirect and require authentication.
	r.Group(func(r chi.Router) {
		r.Use(s.authManager.DomainRedirect())
		r.Use(s.authManager.RequireAuth(http.HandlerFunc(s.handleUIAccessDenied)))

		r.Get("/", s.handleIndex)

		r.Get("/new", s.handleNew)
		r.Post("/new", s.handleNew)

		r.Get("/details/{name}", s.handleDetails)
		r.Post("/details/{name}", s.handleDetails)
		r.Post("/details/{name}/share", s.handleDetailsShare)
		r.Post("/details/{name}/unshare", s.handleDetailsUnshare)
		r.Post("/details/{name}/delete", s.handleDetailsDelete)
		r.Post("/details/{name}/alias", s.handleCreateAlias)
		r.Get("/edit/{name}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/details/"+chi.URLParam(r, "name"), http.StatusMovedPermanently)
		})

		r.Get("/links", s.handleLinks)
		r.Get("/mylinks", s.handleMyLinks)

		r.Get("/help", s.handleHelp)
		r.Get("/help/advanced", s.handleHelpAdvanced)
		r.Get("/help/search", s.handleHelpSearch)

		r.Route("/apikeys", func(r chi.Router) {
			r.Use(s.authManager.RequireAuth(http.HandlerFunc(s.handleForbidden)))
			r.Use(s.authManager.RequireAdmin(http.HandlerFunc(s.handleForbidden)))
			r.Get("/", s.handleAPIKeysPage)
			r.Post("/", s.handleCreateAPIKey)
			r.Post("/{id}/delete", s.handleDeleteAPIKey)
		})

		r.Route("/importexport", func(r chi.Router) {
			r.Use(s.authManager.RequireAuth(http.HandlerFunc(s.handleForbidden)))
			r.Use(s.authManager.RequireAdmin(http.HandlerFunc(s.handleForbidden)))
			r.Get("/", s.handleImportExportPage)
			r.Post("/", s.handleImportUpload)
		})
	})

	// API routes: apply domain redirect and API key auth before enforcement.
	r.Group(func(r chi.Router) {
		r.Use(s.authManager.DomainRedirect())
		r.Use(s.authManager.APIKeyMiddleware(s.lookupAPIKey))

		r.Route("/api", func(r chi.Router) {
			r.Use(s.authManager.RequireAuth(http.HandlerFunc(s.handleForbidden)))

			r.Get("/links", s.handleAPIListLinks)
			r.With(s.authManager.RequireWriteScope()).Post("/links", s.handleAPICreateLink)
			r.Get("/links/{name}", s.handleAPIGetLink)
			r.With(s.authManager.RequireWriteScope()).Patch("/links/{name}", s.handleAPIUpdateLink)
			r.With(s.authManager.RequireWriteScope()).Delete("/links/{name}", s.handleAPIDeleteLink)

			r.Get("/quickname", s.handleQuickName)
			r.Get("/users/search", s.handleAPIUserSearch)

			r.Route("/apikeys", func(r chi.Router) {
				r.Use(s.authManager.RequireAdmin(http.HandlerFunc(s.handleForbidden)))
				r.Use(s.authManager.RequireWriteScope())
				r.Get("/", s.handleAPIListAPIKeys)
				r.Post("/", s.handleAPICreateAPIKey)
				r.Delete("/{id}", s.handleAPIDeleteAPIKey)
			})

			r.With(s.authManager.RequireAdmin(http.HandlerFunc(s.handleForbidden))).Get("/export", s.handleExport)
			r.With(s.authManager.RequireAdmin(http.HandlerFunc(s.handleForbidden)), s.authManager.RequireWriteScope()).Post("/import", s.handleImport)
		})
	})

	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok")) //nolint:errcheck
}

// handleFavicon serves the favicon. When the operator has configured a custom
// favicon_path the file at that path is read from disk; otherwise the embedded
// default SVG favicon is served.
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var contentType string

	if s.cfg.FaviconPath != "" {
		fileData, err := os.ReadFile(s.cfg.FaviconPath)
		if err != nil {
			s.logr(r.Context()).Error("favicon: read custom file", "path", s.cfg.FaviconPath, "error", err)
			http.NotFound(w, r)
			return
		}
		data = fileData
		switch {
		case strings.HasSuffix(s.cfg.FaviconPath, ".svg"):
			contentType = "image/svg+xml"
		case strings.HasSuffix(s.cfg.FaviconPath, ".png"):
			contentType = "image/png"
		default:
			contentType = "image/x-icon"
		}
	} else {
		fileData, err := static.FS.ReadFile("favicon.svg")
		if err != nil {
			s.logr(r.Context()).Error("favicon: read embedded file", "error", err)
			http.NotFound(w, r)
			return
		}
		data = fileData
		contentType = "image/svg+xml"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data) //nolint:errcheck
}
