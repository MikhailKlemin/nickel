//go:build go1.25

package api

import (
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
		logger.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.status,
			"duration", time.Since(start),
		)
	})
}

func recovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered", "recover", rec, "path", r.URL.Path)
				respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
