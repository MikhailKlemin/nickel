//go:build go1.25

package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// wrappedWriter captures the status code so the logger can record it
// after the handler returns.
type wrappedWriter struct {
	http.ResponseWriter
	status int
}

func (w *wrappedWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func logging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &wrappedWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		duration := time.Since(start)
		
		logger.Info("http request",
			slog.Group("request",
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
			),
			slog.Group("response",
				"status", ww.status,
				"duration_ms", duration.Milliseconds(),
			),
		)
	})
}

func recovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// Log limited information to avoid sensitive data exposure
				logger.Error("panic recovered",
					"path", r.URL.Path,
					"method", r.Method,
					"panic_type", fmt.Sprintf("%T", rec),
				)
				respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
