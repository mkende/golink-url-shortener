// Package middleware provides HTTP middleware for the golink server.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// RequestLogger returns a middleware that logs each request using structured logging.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			defer func() {
				logger.Info("request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"bytes", ww.BytesWritten(),
					"duration_ms", time.Since(start).Milliseconds(),
					"remote_addr", r.RemoteAddr,
				)
			}()
			next.ServeHTTP(ww, r)
		})
	}
}
