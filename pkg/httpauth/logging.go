package httpauth

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// requestAttrs is a mutable bag of per-request attributes populated by
// logEnricher after authentication has run. requestLogger stores a pointer in
// the context so the deferred log line can read auth/domain even though the
// enriched request object is not visible from the outer closure.
type requestAttrs struct {
	authSource string
	domain     string
}

type requestAttrsKey struct{}
type contextLoggerKey struct{}

func withRequestAttrs(ctx context.Context, a *requestAttrs) context.Context {
	return context.WithValue(ctx, requestAttrsKey{}, a)
}

func requestAttrsFromContext(ctx context.Context) *requestAttrs {
	a, _ := ctx.Value(requestAttrsKey{}).(*requestAttrs)
	return a
}

// LoggerFromContext returns the request-scoped logger stored in ctx by the
// log-enricher middleware, or fallback when none is present. Use this in
// HTTP handlers to get a logger pre-loaded with auth_source and domain
// attributes.
func LoggerFromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(contextLoggerKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return fallback
}

// responseWriter wraps http.ResponseWriter to capture the status code and
// number of bytes written for access logging.
type responseWriter struct {
	http.ResponseWriter
	status  int
	written int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += n
	return n, err
}

func (rw *responseWriter) statusCode() int {
	if rw.status == 0 {
		return http.StatusOK
	}
	return rw.status
}

// requestLogger returns a middleware that logs each request with structured
// logging. It allocates a requestAttrs bag and stores it in the context so
// that logEnricher (which runs after authentication) can add auth_source and
// domain to the final log line.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attrs := &requestAttrs{}
			r = r.WithContext(withRequestAttrs(r.Context(), attrs))

			rw := &responseWriter{ResponseWriter: w}
			start := time.Now()

			next.ServeHTTP(rw, r)

			args := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.statusCode(),
				"bytes", rw.written,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			}
			if peer := originalRemoteAddr(r.Context()); peer != "" && peer != r.RemoteAddr {
				args = append(args, "peer_addr", peer)
			}
			if attrs.authSource != "" {
				args = append(args, "auth_source", attrs.authSource)
			}
			if attrs.domain != "" {
				args = append(args, "domain", attrs.domain)
			}
			logger.Info("request", args...)
		})
	}
}

// logEnricher returns a middleware that must run after all authentication
// middlewares. It reads the identity (if any) to determine the auth source,
// fills the requestAttrs bag so that the request log line includes those
// attributes, and stores a pre-enriched child logger in context for use by
// handler-level log calls via [LoggerFromContext].
func logEnricher(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authSource := ""
			if id := IdentityFromContext(r.Context()); id != nil {
				authSource = string(id.Source)
			}
			domain := r.Host

			if a := requestAttrsFromContext(r.Context()); a != nil {
				a.authSource = authSource
				a.domain = domain
			}

			child := logger
			if authSource != "" {
				child = child.With("auth_source", authSource)
			}
			if domain != "" {
				child = child.With("domain", domain)
			}
			r = r.WithContext(context.WithValue(r.Context(), contextLoggerKey{}, child))

			next.ServeHTTP(w, r)
		})
	}
}
