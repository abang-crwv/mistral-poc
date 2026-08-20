package server

import (
	"log/slog"
	"net/http"
	"time"
)

// loggingResponseWriter captures the response status so the middleware
// can log it after the handler returns.
type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (lw *loggingResponseWriter) WriteHeader(status int) {
	lw.status = status
	lw.ResponseWriter.WriteHeader(status)
}

// requestLogger logs every request: method, path, status, duration.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(lw, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", lw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
